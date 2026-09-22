package singbox

import (
	"context"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing/common/buf"
	singbufio "github.com/sagernet/sing/common/bufio"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

const (
	maxVisits    = 10000
	maxVisitHost = 253
)

type admission int

const (
	admissionTracked admission = iota
	admissionUntracked
	admissionDenied
)

type pairCounters struct {
	upload   atomic.Int64
	download atomic.Int64
}

type connEntry struct {
	id          uint64
	userID      int64
	nodeID      int64
	ip          string
	connectedAt time.Time
	upload      atomic.Int64
	download    atomic.Int64
	pairUp      *atomic.Int64
	pairDown    *atomic.Int64
}

func (e *connEntry) addUpload(n int64) {
	e.upload.Add(n)
	e.pairUp.Add(n)
}

func (e *connEntry) addDownload(n int64) {
	e.download.Add(n)
	e.pairDown.Add(n)
}

func (e *connEntry) countUp(n int64)   { e.addUpload(n) }
func (e *connEntry) countDown(n int64) { e.addDownload(n) }

type Table struct {
	usersByName map[string]int64
	nodesByTag  map[string]int64
	limits      map[int64]int64
}

func buildTable(users []UserRef) *Table {
	t := &Table{
		usersByName: map[string]int64{},
		nodesByTag:  map[string]int64{},
		limits:      map[int64]int64{},
	}
	for _, u := range users {
		t.usersByName[userName(u.ID)] = u.ID
		t.limits[u.ID] = u.DeviceLimit
		for _, n := range u.Nodes {
			t.nodesByTag[inboundTag(n.Protocol, n.ID)] = n.ID
		}
	}
	return t
}

func (t *Table) userID(name string) (int64, bool) {
	id, ok := t.usersByName[name]
	return id, ok
}

func (t *Table) nodeID(metadata adapter.InboundContext) (int64, bool) {
	id, ok := t.nodesByTag[metadata.Inbound]
	return id, ok
}

func (t *Table) deviceLimit(userID int64) int64 {
	return t.limits[userID]
}

func userName(id int64) string {
	return "u-" + strconv.FormatInt(id, 10)
}

func inboundTag(protocol string, id int64) string {
	return protocol + "-" + strconv.FormatInt(id, 10)
}

type ConnTracker struct {
	table *Table

	mu     sync.Mutex
	nextID uint64
	pairs  map[Pair]*pairCounters
	live   map[uint64]*connEntry

	visitMu       sync.Mutex
	visits        []Visit
	visitsDropped int64
}

func NewConnTracker(users []UserRef) *ConnTracker {
	return &ConnTracker{
		table: buildTable(users),
		pairs: map[Pair]*pairCounters{},
		live:  map[uint64]*connEntry{},
	}
}

func (t *ConnTracker) entry(metadata adapter.InboundContext) (*connEntry, admission) {
	userID, ok := t.table.userID(metadata.User)
	if !ok {
		return nil, admissionUntracked
	}
	nodeID, ok := t.table.nodeID(metadata)
	if !ok {
		return nil, admissionUntracked
	}
	ip := sourceAddr(metadata.Source)

	t.mu.Lock()
	defer t.mu.Unlock()

	if limit := t.table.deviceLimit(userID); limit > 0 && ip != "" {
		alive := map[string]bool{}
		for _, e := range t.live {
			if e.userID == userID {
				alive[e.ip] = true
			}
		}
		if !alive[ip] && int64(len(alive)) >= limit {
			return nil, admissionDenied
		}
	}

	pair := Pair{UserID: userID, NodeID: nodeID}
	counters := t.pairs[pair]
	if counters == nil {
		counters = &pairCounters{}
		t.pairs[pair] = counters
	}
	t.nextID++
	e := &connEntry{
		id:          t.nextID,
		userID:      userID,
		nodeID:      nodeID,
		ip:          ip,
		connectedAt: time.Now(),
		pairUp:      &counters.upload,
		pairDown:    &counters.download,
	}
	t.live[e.id] = e
	t.recordVisit(metadata, userID, nodeID, ip)
	return e, admissionTracked
}

func (t *ConnTracker) recordVisit(metadata adapter.InboundContext, userID, nodeID int64, ip string) {
	host := destinationHost(metadata.Destination)
	if host == "" {
		return
	}
	visit := Visit{
		UserID:   userID,
		NodeID:   nodeID,
		DestHost: host,
		DestPort: int(metadata.Destination.Port),
		Network:  metadata.Network,
		ClientIP: ip,
		At:       time.Now(),
	}

	t.visitMu.Lock()
	defer t.visitMu.Unlock()
	if len(t.visits) >= maxVisits {
		t.visits = t.visits[1:]
		t.visitsDropped++
	}
	t.visits = append(t.visits, visit)
}

func (t *ConnTracker) drainVisits() []Visit {
	t.visitMu.Lock()
	defer t.visitMu.Unlock()
	visits := t.visits
	t.visits = nil
	return visits
}

func (t *ConnTracker) droppedVisits() int64 {
	t.visitMu.Lock()
	defer t.visitMu.Unlock()
	return t.visitsDropped
}

func destinationHost(destination M.Socksaddr) string {
	host := strings.TrimSpace(destination.Fqdn)
	if host == "" && destination.Addr.IsValid() {
		host = destination.Addr.Unmap().String()
	}
	host = strings.ToLower(strings.TrimSpace(host))
	if len(host) > maxVisitHost {
		host = host[:maxVisitHost]
	}
	return host
}

func (t *ConnTracker) release(e *connEntry) {
	t.mu.Lock()
	delete(t.live, e.id)
	t.mu.Unlock()
}

func (t *ConnTracker) RoutedConnection(_ context.Context, conn net.Conn, metadata adapter.InboundContext, _ adapter.Rule, _ adapter.Outbound) net.Conn {
	e, status := t.entry(metadata)
	if status == admissionDenied {
		_ = conn.Close()
		return conn
	}
	if status != admissionTracked {
		return conn
	}
	return &trackedConn{
		ExtendedConn: singbufio.NewExtendedConn(conn),
		entry:        e,
		tracker:      t,
	}
}

func (t *ConnTracker) RoutedPacketConnection(_ context.Context, conn N.PacketConn, metadata adapter.InboundContext, _ adapter.Rule, _ adapter.Outbound) N.PacketConn {
	e, status := t.entry(metadata)
	if status == admissionDenied {
		_ = conn.Close()
		return conn
	}
	if status != admissionTracked {
		return conn
	}
	return &trackedPacketConn{PacketConn: conn, entry: e, tracker: t}
}

func (t *ConnTracker) Snapshot() Snapshot {
	t.mu.Lock()
	defer t.mu.Unlock()

	snap := Snapshot{
		Traffic: make(map[Pair]Traffic, len(t.pairs)),
	}
	for pair, counters := range t.pairs {
		snap.Traffic[pair] = Traffic{
			Upload:   counters.upload.Load(),
			Download: counters.download.Load(),
		}
	}

	type aggregate struct {
		ips    map[string]bool
		online int
	}
	byPair := map[Pair]*aggregate{}
	order := []Pair{}
	for _, e := range t.live {
		pair := Pair{UserID: e.userID, NodeID: e.nodeID}
		agg := byPair[pair]
		if agg == nil {
			agg = &aggregate{ips: map[string]bool{}}
			byPair[pair] = agg
			order = append(order, pair)
		}
		agg.online++
		if e.ip != "" {
			agg.ips[e.ip] = true
		}
	}
	for _, pair := range order {
		agg := byPair[pair]
		ips := make([]string, 0, len(agg.ips))
		for ip := range agg.ips {
			ips = append(ips, ip)
		}
		snap.Devices = append(snap.Devices, Device{
			UserID: pair.UserID,
			NodeID: pair.NodeID,
			IPs:    ips,
			Online: agg.online,
		})
	}
	return snap
}

type trackedConn struct {
	N.ExtendedConn
	entry   *connEntry
	tracker *ConnTracker
	once    sync.Once
}

func (c *trackedConn) Read(p []byte) (int, error) {
	n, err := c.ExtendedConn.Read(p)
	if n > 0 {
		c.entry.addUpload(int64(n))
	}
	return n, err
}

func (c *trackedConn) ReadBuffer(buffer *buf.Buffer) error {
	err := c.ExtendedConn.ReadBuffer(buffer)
	if err == nil && buffer.Len() > 0 {
		c.entry.addUpload(int64(buffer.Len()))
	}
	return err
}

func (c *trackedConn) Write(p []byte) (int, error) {
	n, err := c.ExtendedConn.Write(p)
	if n > 0 {
		c.entry.addDownload(int64(n))
	}
	return n, err
}

func (c *trackedConn) WriteBuffer(buffer *buf.Buffer) error {
	length := int64(buffer.Len())
	err := c.ExtendedConn.WriteBuffer(buffer)
	if err == nil && length > 0 {
		c.entry.addDownload(length)
	}
	return err
}

func (c *trackedConn) Close() error {
	err := c.ExtendedConn.Close()
	c.once.Do(func() { c.tracker.release(c.entry) })
	return err
}

func (c *trackedConn) UnwrapReader() (io.Reader, []N.CountFunc) {
	return c.ExtendedConn, []N.CountFunc{c.entry.countUp}
}

func (c *trackedConn) UnwrapWriter() (io.Writer, []N.CountFunc) {
	return c.ExtendedConn, []N.CountFunc{c.entry.countDown}
}

type trackedPacketConn struct {
	N.PacketConn
	entry   *connEntry
	tracker *ConnTracker
	once    sync.Once
}

func (c *trackedPacketConn) ReadPacket(buffer *buf.Buffer) (M.Socksaddr, error) {
	destination, err := c.PacketConn.ReadPacket(buffer)
	if err == nil && buffer.Len() > 0 {
		c.entry.addUpload(int64(buffer.Len()))
	}
	return destination, err
}

func (c *trackedPacketConn) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	length := int64(buffer.Len())
	err := c.PacketConn.WritePacket(buffer, destination)
	if err == nil && length > 0 {
		c.entry.addDownload(length)
	}
	return err
}

func (c *trackedPacketConn) Close() error {
	err := c.PacketConn.Close()
	c.once.Do(func() { c.tracker.release(c.entry) })
	return err
}

func (c *trackedPacketConn) UnwrapPacketReader() (N.PacketReader, []N.CountFunc) {
	return c.PacketConn, []N.CountFunc{c.entry.countUp}
}

func (c *trackedPacketConn) UnwrapPacketWriter() (N.PacketWriter, []N.CountFunc) {
	return c.PacketConn, []N.CountFunc{c.entry.countDown}
}

func sourceAddr(addr M.Socksaddr) string {
	if addr.Addr.IsValid() {
		return addr.Addr.Unmap().String()
	}
	return addr.AddrString()
}

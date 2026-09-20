package agentruntime

import (
	"context"
	"time"
)

type ipPairKey struct {
	user int64
	node int64
	ip   string
}

type trackedConn struct {
	userID      int64
	node        NodeEntry
	sourceIP    string
	upload      int64
	download    int64
	connectedAt time.Time
}

type TrafficDelta struct {
	UserID   int64
	NodeID   int64
	Upload   int64
	Download int64
}

type SessionSnapshot struct {
	UserID      int64
	NodeID      int64
	IP          string
	Upload      int64
	Download    int64
	ConnectedAt time.Time
	LastSeenAt  time.Time
}

type ClosedLog struct {
	UserID      int64
	NodeID      int64
	IP          string
	Protocol    string
	Upload      int64
	Download    int64
	ConnectedAt time.Time
	ClosedAt    time.Time
}

type PollResult struct {
	Traffic      []TrafficDelta
	Sessions     []SessionSnapshot
	Closed       []ClosedLog
	Unattributed int
	Active       int
}

type Collector struct {
	source  ClashSource
	table   *Table
	tracked map[string]trackedConn
}

func NewCollector(source ClashSource, table *Table) *Collector {
	return &Collector{source: source, table: table, tracked: map[string]trackedConn{}}
}

func (c *Collector) Table() *Table {
	return c.table
}

func (c *Collector) ReplaceTable(table *Table) {
	for id, t := range c.tracked {
		if !table.pairs[pairKey{user: t.userID, node: t.node.ID}] {
			delete(c.tracked, id)
		}
	}
	c.table = table
}

func (c *Collector) Poll(ctx context.Context, now time.Time) (*PollResult, error) {
	snapshot, err := c.source.Connections(ctx)
	if err != nil {
		return nil, err
	}

	res := &PollResult{}
	current := map[string]attributedConn{}
	pairTraffic := map[pairKey]*TrafficDelta{}

	for _, raw := range snapshot.Connections {
		attr, ok := c.table.attribute(raw)
		if !ok {
			res.Unattributed++
			continue
		}
		res.Active++
		current[attr.id] = *attr

		prev, tracked := c.tracked[attr.id]
		delta := connDelta(*attr, prev, tracked)
		key := pairKey{user: attr.userID, node: attr.node.ID}
		tot, ok := pairTraffic[key]
		if !ok {
			tot = &TrafficDelta{UserID: key.user, NodeID: key.node}
			pairTraffic[key] = tot
		}
		tot.Upload += delta.upload
		tot.Download += delta.download

		c.tracked[attr.id] = trackedConn{
			userID:      attr.userID,
			node:        attr.node,
			sourceIP:    attr.sourceIP,
			upload:      attr.upload,
			download:    attr.download,
			connectedAt: attr.connected,
		}
	}

	for id, prev := range c.tracked {
		if _, live := current[id]; live {
			continue
		}
		key := pairKey{user: prev.userID, node: prev.node.ID}
		tot, ok := pairTraffic[key]
		if !ok {
			tot = &TrafficDelta{UserID: key.user, NodeID: key.node}
			pairTraffic[key] = tot
		}
		tot.Upload += prev.upload
		tot.Download += prev.download
		if prev.sourceIP != "" {
			res.Closed = append(res.Closed, ClosedLog{
				UserID:      prev.userID,
				NodeID:      prev.node.ID,
				IP:          prev.sourceIP,
				Protocol:    prev.node.Protocol,
				Upload:      prev.upload,
				Download:    prev.download,
				ConnectedAt: prev.connectedAt,
				ClosedAt:    now,
			})
		}
		delete(c.tracked, id)
	}

	for _, d := range pairTraffic {
		if d.Upload != 0 || d.Download != 0 {
			res.Traffic = append(res.Traffic, *d)
		}
	}
	res.Sessions = mergeSessions(current, now)
	return res, nil
}

type trafficDeltaTotals struct {
	upload   int64
	download int64
}

func connDelta(attr attributedConn, prev trackedConn, tracked bool) trafficDeltaTotals {
	up, down := attr.upload, attr.download
	if tracked {
		up -= prev.upload
		down -= prev.download
	}
	if up < 0 {
		up = 0
	}
	if down < 0 {
		down = 0
	}
	return trafficDeltaTotals{upload: up, download: down}
}

func mergeSessions(current map[string]attributedConn, now time.Time) []SessionSnapshot {
	type agg struct {
		userID, nodeID int64
		ip             string
		upload, down   int64
		connectedAt    time.Time
	}
	merged := map[ipPairKey]*agg{}
	for _, a := range current {
		if a.sourceIP == "" {
			continue
		}
		key := ipPairKey{user: a.userID, node: a.node.ID, ip: a.sourceIP}
		entry, ok := merged[key]
		if !ok {
			entry = &agg{userID: a.userID, nodeID: a.node.ID, ip: a.sourceIP, connectedAt: a.connected}
			merged[key] = entry
		}
		entry.upload += a.upload
		entry.down += a.download
		if a.connected.Before(entry.connectedAt) {
			entry.connectedAt = a.connected
		}
	}
	out := make([]SessionSnapshot, 0, len(merged))
	for _, e := range merged {
		out = append(out, SessionSnapshot{
			UserID:      e.userID,
			NodeID:      e.nodeID,
			IP:          e.ip,
			Upload:      e.upload,
			Download:    e.down,
			ConnectedAt: e.connectedAt,
			LastSeenAt:  now,
		})
	}
	return out
}

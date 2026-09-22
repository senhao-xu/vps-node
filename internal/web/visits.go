package web

import (
	"net/http"
	"strconv"
	"time"

	"vps-node/internal/repo"
)

func queryID(r *http.Request, name string) (int64, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id < 1 {
		return 0, errInvalid("invalid " + name)
	}
	return id, nil
}

func visitFilterFromQuery(r *http.Request) (repo.VisitFilter, error) {
	userID, err := queryID(r, "user_id")
	if err != nil {
		return repo.VisitFilter{}, err
	}
	serverID, err := queryID(r, "server_id")
	if err != nil {
		return repo.VisitFilter{}, err
	}
	nodeID, err := queryID(r, "node_id")
	if err != nil {
		return repo.VisitFilter{}, err
	}
	from, err := parseTimeParam(r, "from")
	if err != nil {
		return repo.VisitFilter{}, err
	}
	to, err := parseTimeParam(r, "to")
	if err != nil {
		return repo.VisitFilter{}, err
	}
	return repo.VisitFilter{
		UserID:   userID,
		ServerID: serverID,
		NodeID:   nodeID,
		Host:     r.URL.Query().Get("host"),
		ClientIP: r.URL.Query().Get("client_ip"),
		From:     from,
		To:       to,
	}, nil
}

func visitDTOs(visits []repo.VisitRecord) []visitDTO {
	items := make([]visitDTO, 0, len(visits))
	for _, v := range visits {
		items = append(items, toVisitDTO(v))
	}
	return items
}

func (h *Handler) handleVisitList(w http.ResponseWriter, r *http.Request) {
	page, err := parsePageQuery(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	filter, err := visitFilterFromQuery(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	filter.Page = page.Page
	filter.PageSize = page.PageSize

	visits, total, err := h.repo.ListVisits(r.Context(), filter)
	if err != nil {
		writeErr(w, err)
		return
	}
	writePage(w, visitDTOs(visits), total, page)
}

func (h *Handler) handleVisitTop(w http.ResponseWriter, r *http.Request) {
	filter, err := visitFilterFromQuery(r)
	if err != nil {
		writeErr(w, err)
		return
	}

	days := 7
	if raw := r.URL.Query().Get("days"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			writeErr(w, errInvalid("invalid days"))
			return
		}
		days = n
	}
	if days > 365 {
		days = 365
	}

	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			writeErr(w, errInvalid("invalid limit"))
			return
		}
		limit = n
	}
	if limit > 200 {
		limit = 200
	}

	since := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -(days - 1))
	hosts, err := h.repo.TopVisitHosts(r.Context(), filter, since.Unix(), limit)
	if err != nil {
		writeErr(w, err)
		return
	}
	items := make([]topHostDTO, 0, len(hosts))
	for _, host := range hosts {
		items = append(items, topHostDTO{DestHost: host.DestHost, Hits: host.Hits})
	}
	writePage(w, items, int64(len(items)), pageQuery{Page: 1, PageSize: limit})
}

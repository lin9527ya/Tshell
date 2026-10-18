package main

import (
	"encoding/json"
	"net"
	"net/http"
	"time"
)

// 健康检查：/healthz 返回连接数/流数/计数，便于运维探活与容量观察。
// 只绑回环（配置里默认 127.0.0.1），不对外暴露中继的内部状态。
type healthServer struct {
	tcp     *tcpRelay
	udp     *udpRelay
	dns     *dnsRelay
	started time.Time
	version string
}

func (h *healthServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		total, ips := h.tcp.limiter.counts()
		resp := map[string]interface{}{
			"status":     "ok",
			"version":    h.version,
			"uptime_s":   int64(time.Since(h.started).Seconds()),
			"tcp_active": h.tcp.active.Load(),
			"tcp_total":  total,
			"tcp_ips":    ips,
			"accepted":   h.tcp.accepted.Load(),
			"rejected":   h.tcp.rejected.Load(),
		}
		// 上游健康度：多上游部署下，运维最需要一眼看出"有几个上游还活着"
		if h.tcp.pool != nil {
			resp["upstreams"] = h.tcp.pool.targets
			resp["upstreams_healthy"] = h.tcp.pool.healthy()
			resp["upstreams_picked"] = h.tcp.pool.picked.Load()
			resp["upstreams_skipped"] = h.tcp.pool.skipped.Load()
		}
		if h.udp != nil {
			resp["udp_flows"] = h.udp.count.Load()
			resp["udp_in"] = h.udp.datagramsIn.Load()
			resp["udp_out"] = h.udp.datagramsOut.Load()
			resp["udp_rejected"] = h.udp.rejected.Load()
		}
		if h.dns != nil {
			resp["dns_queries"] = h.dns.queries.Load()
			resp["dns_errors"] = h.dns.errors.Load()
			resp["dns_tcp_tries"] = h.dns.tcpTries.Load()
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	return mux
}

// serve 在已有监听器上起健康检查；调用方负责关闭
func (h *healthServer) serve(ln net.Listener) *http.Server {
	srv := &http.Server{Handler: h.handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	return srv
}

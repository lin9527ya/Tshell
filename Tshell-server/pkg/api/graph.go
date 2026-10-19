package api

import (
	"BackendTemplate/pkg/database"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// ============ Graph 关系图 API（CS Graph 页面数据源） ============
// 数据全部来自现有表，无 schema 变更：
//   - nodes: server / listener / client / tunnel / target
//   - edges: server→listener、client→listener(按 Client.Listener 类型)、
//     client→tunnel(按 TunnelProxy.ClientUID)、tunnel→target

// GraphNode 图节点
type GraphNode struct {
	ID       string            `json:"id"`
	Type     string            `json:"type"` // server | listener | client | tunnel | target
	Label    string            `json:"label"`
	SubLabel string            `json:"sub_label,omitempty"`
	Online   string            `json:"online,omitempty"` // "1"|"2" (client 节点)
	Meta     map[string]string `json:"meta,omitempty"`
}

// GraphEdge 图边
type GraphEdge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Label  string `json:"label,omitempty"`
}

// GetGraphData 返回关系图数据（operatorOnly，经 main.go 路由中间件控制）
func GetGraphData(c *gin.Context) {
	nodes := []GraphNode{}
	edges := []GraphEdge{}

	// 1. server 节点
	nodes = append(nodes, GraphNode{
		ID:    "server",
		Type:  "server",
		Label: "Tshell Server",
	})

	// 2. listener 节点 + server→listener 边
	var listeners []database.Listener
	listeners, _ = database.Listeners().List()
	listenerSet := map[string]string{} // type → node id
	for _, l := range listeners {
		if l.Type == "" {
			continue
		}
		id := "listener_" + l.Type
		if l.ListenAddress != "" {
			id = "listener_" + l.Type + "_" + strings.ReplaceAll(strings.ReplaceAll(l.ListenAddress, ".", "_"), ":", "_")
		}
		addr := l.ListenAddress
		if addr == "" {
			addr = l.ConnectAddress
		}
		// 备注在图上别处看不到（节点只放类型+地址），放进 meta 供详情面板展示，
		// 同时作为 server→listener 边的标签 —— 边越长越难追，标签是唯一的线索。
		var meta map[string]string
		if l.Note != "" {
			meta = map[string]string{"note": l.Note}
		}
		nodes = append(nodes, GraphNode{
			ID:       id,
			Type:     "listener",
			Label:    l.Type,
			SubLabel: addr,
			Meta:     meta,
		})
		edges = append(edges, GraphEdge{ID: "e_srv_" + id, Source: "server", Target: id, Label: l.Note})
		if _, ok := listenerSet[l.Type]; !ok {
			listenerSet[l.Type] = id
		}
	}

	// 3. client 节点 + client→listener 边（按类型回退）
	var clients []database.Clients
	clients, _ = database.Clients_().All()
	for _, cl := range clients {
		if cl.Uid == "" {
			continue
		}
		label := cl.Computer
		if label == "" {
			label = cl.Username
		}
		if label == "" {
			label = cl.Uid[:min(8, len(cl.Uid))]
		}
		nodes = append(nodes, GraphNode{
			ID:       "client_" + cl.Uid,
			Type:     "client",
			Label:    label,
			SubLabel: cl.ExternalIP,
			Online:   cl.Online,
			Meta: map[string]string{
				"uid":      cl.Uid,
				"username": cl.Username,
				"internal": cl.InternalIP,
				"os":       cl.Arch,
			},
		})
		if lid, ok := listenerSet[cl.Listener]; ok {
			edges = append(edges, GraphEdge{
				ID:     "e_" + cl.Uid + "_" + lid,
				Source: "client_" + cl.Uid,
				Target: lid,
			})
		}
	}

	// 4. tunnel 节点 + client→tunnel + tunnel→target 边
	var tunnels []database.TunnelProxy
	tunnels, _ = database.Tunnels().List()
	targetSet := map[string]string{}
	for _, t := range tunnels {
		tid := "tunnel_" + strconv.Itoa(int(t.ID))
		nodes = append(nodes, GraphNode{
			ID:       tid,
			Type:     "tunnel",
			Label:    t.Protocol,
			SubLabel: t.TargetAddress,
		})
		if t.ClientUID != "" {
			edges = append(edges, GraphEdge{
				ID:     "e_" + t.ClientUID + "_" + tid,
				Source: "client_" + t.ClientUID,
				Target: tid,
				Label:  t.Note,
			})
		}
		if t.TargetAddress != "" {
			targetID, ok := targetSet[t.TargetAddress]
			if !ok {
				targetID = "target_" + strings.ReplaceAll(strings.ReplaceAll(t.TargetAddress, ".", "_"), ":", "_")
				targetSet[t.TargetAddress] = targetID
				nodes = append(nodes, GraphNode{
					ID:       targetID,
					Type:     "target",
					Label:    t.TargetAddress,
					SubLabel: "target",
				})
			}
			edges = append(edges, GraphEdge{
				ID:     "e_" + tid + "_" + targetID,
				Source: tid,
				Target: targetID,
				Label:  t.Note,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status": http.StatusOK,
		"data": gin.H{
			"nodes": nodes,
			"edges": edges,
		},
	})
}

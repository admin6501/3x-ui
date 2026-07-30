package service

import "github.com/mhsanaei/3x-ui/v3/internal/database/model"

// NodeView is the read contract for a node.
//
// A node's API token is full control of that node's own panel, so it must
// never leave this one. The node list is reachable by every signed-in role —
// the Inbounds, Hosts and Clients pages all read it to name the node an
// inbound lives on — so returning the raw model would hand a manager, a
// reseller, or even a read-only account the credentials to administer every
// node. The token is replaced by a presence flag, which is all the edit form
// ever needed it for.
type NodeView struct {
	*model.Node
	// ApiToken shadows the embedded field so it serialises empty regardless of
	// what is stored. Redacting on the way out rather than not loading it
	// keeps every internal caller (reconcile, heartbeat, traffic sync) working
	// against the real model.
	ApiToken string `json:"apiToken"`
	// HasApiToken tells the edit form a token is already stored, so it can
	// leave the field blank and mean "keep it".
	HasApiToken bool `json:"hasApiToken"`
}

func newNodeView(n *model.Node) *NodeView {
	if n == nil {
		return nil
	}
	return &NodeView{
		Node:        n,
		ApiToken:    "",
		HasApiToken: n.ApiToken != "",
	}
}

// NodeViews redacts a slice of nodes for a read endpoint.
func NodeViews(nodes []*model.Node) []*NodeView {
	out := make([]*NodeView, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, newNodeView(n))
	}
	return out
}

// NodeViewOf redacts a single node for a read endpoint.
func NodeViewOf(n *model.Node) *NodeView { return newNodeView(n) }

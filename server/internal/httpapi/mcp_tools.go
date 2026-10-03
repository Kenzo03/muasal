package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// mcpServer builds the tool set for one request. ponytail: schemas are
// inferred per request; cache the server per token if profiles show it.
func (s *Server) mcpServer(c *apiCaller) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "muasal", Version: "0.1"}, nil)

	mcp.AddTool(srv, &mcp.Tool{Name: "get_project", Description: "A project's statuses (with category todo, in_progress, done or cancelled), menu tree (flat node list with parent ids; tickets attach to menus by id), clients and assignees. Call it first for the ids the other tools take."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in projectIn) (*mcp.CallToolResult, any, error) {
			p := "/projects/" + url.PathEscape(in.Project)
			out := map[string]json.RawMessage{}
			for name, path := range map[string]string{"project": p, "statuses": p + "/statuses", "menus": p + "/nodes", "clients": p + "/clients", "assignees": p + "/assignees"} {
				var raw json.RawMessage
				if _, err := c.call(ctx, http.MethodGet, path, nil, nil, &raw); err != nil {
					return nil, nil, err
				}
				out[name] = raw
			}
			return jsonResult(out)
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "list_tickets", Description: "Tickets in a project that you may see, newest change first unless sort says otherwise. Pass next_cursor back as cursor for the next page."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in listIn) (*mcp.CallToolResult, any, error) {
			q := url.Values{}
			setStr := func(k string, v *string) {
				if v != nil && *v != "" {
					q.Set(k, *v)
				}
			}
			setInt := func(k string, v *int64) {
				if v != nil {
					q.Set(k, strconv.FormatInt(*v, 10))
				}
			}
			setBool := func(k string, v *bool) {
				if v != nil {
					q.Set(k, strconv.FormatBool(*v))
				}
			}
			setStr("q", in.Q)
			setStr("type", in.Type)
			setStr("sort", in.Sort)
			setStr("cursor", in.Cursor)
			setInt("status_id", in.StatusID)
			setInt("client_id", in.ClientID)
			setInt("assignee_id", in.AssigneeID)
			setInt("node_id", in.NodeID)
			setBool("open", in.Open)
			setBool("mine", in.Mine)
			limit := 50
			if in.Limit != nil && *in.Limit > 0 {
				limit = min(*in.Limit, 200)
			}
			q.Set("limit", strconv.Itoa(limit))
			var raw json.RawMessage
			if _, err := c.call(ctx, http.MethodGet, "/projects/"+url.PathEscape(in.Project)+"/tickets?"+q.Encode(), nil, nil, &raw); err != nil {
				return nil, nil, err
			}
			return jsonResult(raw)
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "get_ticket", Description: "One ticket by key, e.g. HRIS-12, with its decision record and version. Pass version to update_ticket to avoid overwriting someone else's change."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in keyIn) (*mcp.CallToolResult, any, error) {
			var raw json.RawMessage
			if _, err := c.call(ctx, http.MethodGet, "/tickets/"+url.PathEscape(in.Key), nil, nil, &raw); err != nil {
				return nil, nil, err
			}
			return jsonResult(raw)
		})

	return srv
}

type projectIn struct {
	Project string `json:"project" jsonschema:"the project key, e.g. HRIS"`
}

type keyIn struct {
	Key string `json:"key" jsonschema:"the ticket key, e.g. HRIS-12"`
}

type listIn struct {
	Project    string  `json:"project" jsonschema:"the project key, e.g. HRIS"`
	Q          *string `json:"q,omitempty" jsonschema:"title words or a ticket key"`
	StatusID   *int64  `json:"status_id,omitempty" jsonschema:"only this status (ids from get_project)"`
	Open       *bool   `json:"open,omitempty" jsonschema:"true for open tickets only"`
	Type       *string `json:"type,omitempty" jsonschema:"bug, change_request or feature"`
	ClientID   *int64  `json:"client_id,omitempty" jsonschema:"only this client's tickets"`
	AssigneeID *int64  `json:"assignee_id,omitempty" jsonschema:"only tickets assigned to this user"`
	Mine       *bool   `json:"mine,omitempty" jsonschema:"true for tickets assigned to you"`
	NodeID     *int64  `json:"node_id,omitempty" jsonschema:"only tickets on this menu"`
	Sort       *string `json:"sort,omitempty" jsonschema:"updated, created, key, priority or due"`
	Limit      *int    `json:"limit,omitempty" jsonschema:"page size, 1 to 200, default 50"`
	Cursor     *string `json:"cursor,omitempty" jsonschema:"next_cursor from the previous page"`
}

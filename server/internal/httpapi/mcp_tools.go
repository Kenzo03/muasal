package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	openapi_types "github.com/oapi-codegen/runtime/types"
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

	mcp.AddTool(srv, &mcp.Tool{Name: "create_ticket", Description: "File a ticket. node_ids are menu ids from get_project. Leave client_id out for core work that serves every client. Send the same idempotency_key when retrying."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in createIn) (*mcp.CallToolResult, any, error) {
			body, err := bodyOf(in, "project", "idempotency_key")
			if err != nil {
				return nil, nil, err
			}
			var h map[string]string
			if in.IdempotencyKey != nil && *in.IdempotencyKey != "" {
				h = map[string]string{"Idempotency-Key": *in.IdempotencyKey}
			}
			var raw json.RawMessage
			if _, err := c.call(ctx, http.MethodPost, "/projects/"+url.PathEscape(in.Project)+"/tickets", h, body, &raw); err != nil {
				return nil, nil, err
			}
			return jsonResult(raw)
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "update_ticket", Description: "Change a ticket's fields. Only the fields you pass change. Pass version from get_ticket to fail instead of overwriting a newer change. Status changes go through transition_ticket."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in updateIn) (*mcp.CallToolResult, any, error) {
			path := "/tickets/" + url.PathEscape(in.Key)
			var cur Ticket
			if _, err := c.call(ctx, http.MethodGet, path, nil, nil, &cur); err != nil {
				return nil, nil, err
			}
			up := TicketUpdate{Type: cur.Type, Title: cur.Title, Reason: &cur.Reason, Description: &cur.Description,
				Priority: &cur.Priority, DueDate: cur.DueDate, NodeIds: make([]int64, len(cur.Nodes))}
			for i, n := range cur.Nodes {
				up.NodeIds[i] = n.Id
			}
			if cur.Client != nil {
				up.ClientId = &cur.Client.Id
			}
			if cur.Assignee != nil {
				up.AssigneeId = &cur.Assignee.Id
			}
			if cur.Requester.Kind == TicketRequesterKindContact {
				up.RequesterContactId = &cur.Requester.Id
			} else {
				up.RequesterUserId = &cur.Requester.Id
			}
			if in.Type != nil {
				up.Type = TicketType(*in.Type)
			}
			if in.Title != nil {
				up.Title = *in.Title
			}
			if in.NodeIDs != nil {
				up.NodeIds = *in.NodeIDs
			}
			if in.ClientID != nil {
				up.ClientId = in.ClientID
			}
			if in.RequesterContactID != nil {
				up.RequesterContactId, up.RequesterUserId = in.RequesterContactID, nil
			}
			if in.RequesterUserID != nil {
				up.RequesterUserId, up.RequesterContactId = in.RequesterUserID, nil
			}
			if in.Reason != nil {
				up.Reason = in.Reason
			}
			if in.Description != nil {
				up.Description = in.Description
			}
			if in.AssigneeID != nil {
				up.AssigneeId = in.AssigneeID
			}
			if in.Priority != nil {
				p := Priority(*in.Priority)
				up.Priority = &p
			}
			if in.DueDate != nil {
				var d openapi_types.Date
				if err := d.UnmarshalText([]byte(*in.DueDate)); err != nil {
					return nil, nil, fmt.Errorf("due_date must be YYYY-MM-DD: %w", err)
				}
				up.DueDate = &d
			}
			version := cur.Version
			if in.Version != nil {
				version = *in.Version
			}
			var raw json.RawMessage
			if _, err := c.call(ctx, http.MethodPut, path, map[string]string{"If-Match": fmt.Sprintf(`"%d"`, version)}, up, &raw); err != nil {
				return nil, nil, err
			}
			return jsonResult(raw)
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "transition_ticket", Description: "Move a ticket to a status (ids from get_project). Moving to a done or cancelled status closes it and needs reason, at least one menu and decision {what_changed, why}. Leaving a closed status reopens it."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in transitionIn) (*mcp.CallToolResult, any, error) {
			req := TransitionRequest{StatusId: in.StatusID, Reason: in.Reason, NodeIds: in.NodeIDs}
			if in.Decision != nil {
				req.Decision = &DecisionInput{WhatChanged: in.Decision.WhatChanged, Why: in.Decision.Why, Alternatives: in.Decision.Alternatives}
			}
			var raw json.RawMessage
			if _, err := c.call(ctx, http.MethodPost, "/tickets/"+url.PathEscape(in.Key)+"/transition", nil, req, &raw); err != nil {
				return nil, nil, err
			}
			return jsonResult(raw)
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "cancel_ticket", Description: "Muasal never deletes tickets: this closes one as Cancelled and records why, so the history stays. reason and why are required."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in cancelIn) (*mcp.CallToolResult, any, error) {
			path := "/tickets/" + url.PathEscape(in.Key)
			var cur Ticket
			if _, err := c.call(ctx, http.MethodGet, path, nil, nil, &cur); err != nil {
				return nil, nil, err
			}
			var statuses StatusList
			if _, err := c.call(ctx, http.MethodGet, "/projects/"+url.PathEscape(cur.ProjectKey)+"/statuses", nil, nil, &statuses); err != nil {
				return nil, nil, err
			}
			var cancelled *Status
			for i := range statuses.Items {
				if statuses.Items[i].Category == StatusCategoryCancelled {
					cancelled = &statuses.Items[i]
					break
				}
			}
			if cancelled == nil {
				return nil, nil, errors.New("no_cancelled_status: this project has no status in the cancelled category")
			}
			what := "Cancelled; nothing changed."
			if in.WhatChanged != nil && *in.WhatChanged != "" {
				what = *in.WhatChanged
			}
			nodes := in.NodeIDs
			if nodes == nil {
				ids := make([]int64, len(cur.Nodes))
				for i, n := range cur.Nodes {
					ids[i] = n.Id
				}
				nodes = &ids
			}
			req := TransitionRequest{StatusId: cancelled.Id, Reason: &in.Reason, NodeIds: nodes,
				Decision: &DecisionInput{WhatChanged: what, Why: in.Why, Alternatives: in.Alternatives}}
			var raw json.RawMessage
			if _, err := c.call(ctx, http.MethodPost, path+"/transition", nil, req, &raw); err != nil {
				return nil, nil, err
			}
			return jsonResult(raw)
		})

	addTreeTools(srv, c)
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

type createIn struct {
	Project        string  `json:"project" jsonschema:"the project key, e.g. HRIS"`
	Type           string  `json:"type" jsonschema:"bug, change_request or feature"`
	Title          string  `json:"title" jsonschema:"at most 200 characters"`
	NodeIDs        []int64 `json:"node_ids" jsonschema:"menu ids from get_project"`
	ClientID       *int64  `json:"client_id,omitempty" jsonschema:"the client asking; leave out for core work"`
	Reason         *string `json:"reason,omitempty" jsonschema:"why the change is needed"`
	Description    *string `json:"description,omitempty" jsonschema:"Markdown details"`
	AssigneeID     *int64  `json:"assignee_id,omitempty"`
	Priority       *string `json:"priority,omitempty" jsonschema:"low, medium, high or urgent"`
	DueDate        *string `json:"due_date,omitempty" jsonschema:"YYYY-MM-DD"`
	StatusID       *int64  `json:"status_id,omitempty" jsonschema:"an open status; the project default if left out"`
	IdempotencyKey *string `json:"idempotency_key,omitempty" jsonschema:"repeat it when retrying to avoid a duplicate ticket"`
}

type updateIn struct {
	Key                string   `json:"key" jsonschema:"the ticket key, e.g. HRIS-12"`
	Type               *string  `json:"type,omitempty" jsonschema:"bug, change_request or feature"`
	Title              *string  `json:"title,omitempty"`
	NodeIDs            *[]int64 `json:"node_ids,omitempty" jsonschema:"replaces the menus"`
	ClientID           *int64   `json:"client_id,omitempty"`
	RequesterContactID *int64   `json:"requester_contact_id,omitempty"`
	RequesterUserID    *int64   `json:"requester_user_id,omitempty"`
	Reason             *string  `json:"reason,omitempty"`
	Description        *string  `json:"description,omitempty"`
	AssigneeID         *int64   `json:"assignee_id,omitempty"`
	Priority           *string  `json:"priority,omitempty" jsonschema:"low, medium, high or urgent"`
	DueDate            *string  `json:"due_date,omitempty" jsonschema:"YYYY-MM-DD"`
	Version            *int32   `json:"version,omitempty" jsonschema:"the version you read; the update fails if the ticket changed since"`
}

type decisionIn struct {
	WhatChanged  string  `json:"what_changed" jsonschema:"what changed in the product"`
	Why          string  `json:"why" jsonschema:"why it changed"`
	Alternatives *string `json:"alternatives,omitempty" jsonschema:"what was considered and rejected"`
}

type transitionIn struct {
	Key      string      `json:"key" jsonschema:"the ticket key, e.g. HRIS-12"`
	StatusID int64       `json:"status_id" jsonschema:"the target status id from get_project"`
	Reason   *string     `json:"reason,omitempty" jsonschema:"closing only: replaces the ticket's reason"`
	NodeIDs  *[]int64    `json:"node_ids,omitempty" jsonschema:"closing only: replaces the menus"`
	Decision *decisionIn `json:"decision,omitempty" jsonschema:"closing only: the decision record"`
}

type cancelIn struct {
	Key          string   `json:"key" jsonschema:"the ticket key, e.g. HRIS-12"`
	Reason       string   `json:"reason" jsonschema:"why the ticket existed"`
	Why          string   `json:"why" jsonschema:"why it is cancelled"`
	WhatChanged  *string  `json:"what_changed,omitempty" jsonschema:"default: Cancelled; nothing changed."`
	Alternatives *string  `json:"alternatives,omitempty"`
	NodeIDs      *[]int64 `json:"node_ids,omitempty" jsonschema:"default: the ticket's menus"`
}

// bodyOf turns a tool input into an API body without the tool-only fields.
func bodyOf(v any, drop ...string) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	for _, k := range drop {
		delete(m, k)
	}
	return m, nil
}

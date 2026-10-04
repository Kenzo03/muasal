package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// addTreeTools lets project admins shape the module tree (FSD §7.5). There is
// no delete: archive a node with update_node, as the web app does.
func addTreeTools(srv *mcp.Server, c *apiCaller) {
	mcp.AddTool(srv, &mcp.Tool{Name: "create_node", Description: "Project admins only. Add a module or menu to a project's tree, last among its siblings. Leave parent_id out for the top level. Menus are what tickets attach to."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in nodeCreateIn) (*mcp.CallToolResult, any, error) {
			body, err := bodyOf(in, "project")
			if err != nil {
				return nil, nil, err
			}
			var raw json.RawMessage
			if _, err := c.call(ctx, http.MethodPost, "/projects/"+url.PathEscape(in.Project)+"/nodes", nil, body, &raw); err != nil {
				return nil, nil, err
			}
			return jsonResult(raw)
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "update_node", Description: "Project admins only. Change a module or menu (id from get_project): only the fields you pass change. move {parent_id, position} moves it; leave parent_id out to move it to the top level. archived true archives it, false restores it."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in nodeUpdateIn) (*mcp.CallToolResult, any, error) {
			body, err := bodyOf(in, "id")
			if err != nil {
				return nil, nil, err
			}
			var raw json.RawMessage
			if _, err := c.call(ctx, http.MethodPatch, "/nodes/"+strconv.FormatInt(in.ID, 10), nil, body, &raw); err != nil {
				return nil, nil, err
			}
			return jsonResult(raw)
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "import_tree", Description: "Project admins only. Import a module-tree CSV, e.g. header path,type,code,client_scope,clients,aliases and rows like \"HR > Overtime Approval,menu,HR.OT,shared,,\". Rows match nodes by code, else by path; nothing is deleted. It only previews unless apply is true; any row problem stops the whole import."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in importTreeIn) (*mcp.CallToolResult, any, error) {
			var buf bytes.Buffer
			mw := multipart.NewWriter(&buf)
			fw, err := mw.CreateFormFile("file", "tree.csv")
			if err != nil {
				return nil, nil, err
			}
			if _, err := fw.Write([]byte(in.CSV)); err != nil {
				return nil, nil, err
			}
			if err := mw.WriteField("dry_run", strconv.FormatBool(!in.Apply)); err != nil {
				return nil, nil, err
			}
			if err := mw.Close(); err != nil {
				return nil, nil, err
			}
			var raw json.RawMessage
			if _, err := c.call(ctx, http.MethodPost, "/projects/"+url.PathEscape(in.Project)+"/nodes/import", nil,
				rawBody{contentType: mw.FormDataContentType(), data: buf.Bytes()}, &raw); err != nil {
				return nil, nil, err
			}
			return jsonResult(raw)
		})
}

type nodeCreateIn struct {
	Project        string   `json:"project" jsonschema:"the project key, e.g. HRIS"`
	Type           string   `json:"type" jsonschema:"module or menu"`
	Name           string   `json:"name" jsonschema:"at most 200 characters"`
	ParentID       *int64   `json:"parent_id,omitempty" jsonschema:"the parent node id; leave out for the top level"`
	Code           *string  `json:"code,omitempty" jsonschema:"a short unique code, e.g. HR.OT"`
	Aliases        []string `json:"aliases,omitempty" jsonschema:"other names people use, at most 20"`
	Description    *string  `json:"description,omitempty"`
	ClientSpecific *bool    `json:"client_specific,omitempty" jsonschema:"true when only some clients have it"`
	ClientIDs      []int64  `json:"client_ids,omitempty" jsonschema:"the clients that have it, when client_specific"`
}

type nodeMoveIn struct {
	ParentID *int64 `json:"parent_id,omitempty" jsonschema:"the new parent; leave out for the top level"`
	Position *int32 `json:"position,omitempty" jsonschema:"index among the new siblings; leave out to put it last"`
}

type nodeUpdateIn struct {
	ID             int64       `json:"id" jsonschema:"the node id from get_project"`
	Name           *string     `json:"name,omitempty"`
	Type           *string     `json:"type,omitempty" jsonschema:"module or menu"`
	Code           *string     `json:"code,omitempty" jsonschema:"an empty code clears it"`
	Aliases        []string    `json:"aliases,omitempty" jsonschema:"replaces the aliases"`
	Description    *string     `json:"description,omitempty"`
	ClientSpecific *bool       `json:"client_specific,omitempty" jsonschema:"when given, client_ids replaces the node's clients"`
	ClientIDs      []int64     `json:"client_ids,omitempty"`
	Move           *nodeMoveIn `json:"move,omitempty"`
	Archived       *bool       `json:"archived,omitempty" jsonschema:"true archives, false restores"`
}

type importTreeIn struct {
	Project string `json:"project" jsonschema:"the project key, e.g. HRIS"`
	CSV     string `json:"csv" jsonschema:"the CSV text, path shape or adjacency shape, at most 5,000 rows"`
	Apply   bool   `json:"apply,omitempty" jsonschema:"true applies the import; otherwise it only previews"`
}

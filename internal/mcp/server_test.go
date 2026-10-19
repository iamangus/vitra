package mcp

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/iamangus/vitra/internal"
	"github.com/mark3labs/mcp-go/client"
	protocol "github.com/mark3labs/mcp-go/mcp"
)

func TestProtocolToolsAndContainment(t *testing.T) {
	fs := internal.NewFileSystem(t.TempDir())
	fs.SetSkillsDir("skills")
	if err := fs.BuildIndex(); err != nil {
		t.Fatal(err)
	}
	server := NewToolsServer(fs)
	httpServer := httptest.NewServer(server)
	defer httpServer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, err := client.NewStreamableHttpClient(httpServer.URL + "/mcp")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Start(ctx); err != nil {
		t.Fatal(err)
	}
	var init protocol.InitializeRequest
	init.Params.ProtocolVersion = protocol.LATEST_PROTOCOL_VERSION
	init.Params.ClientInfo = protocol.Implementation{Name: "test", Version: "1"}
	if _, err := c.Initialize(ctx, init); err != nil {
		t.Fatal(err)
	}
	tools, err := c.ListTools(ctx, protocol.ListToolsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 19 {
		t.Fatal(len(tools.Tools))
	}
	for _, tool := range tools.Tools {
		if tool.Name == "search_wiki" {
			for _, name := range []string{"type", "tag", "tags", "limit"} {
				if _, ok := tool.InputSchema.Properties[name]; !ok {
					t.Fatal("missing schema", name)
				}
			}
		}
	}
	call := func(name string, args map[string]interface{}) bool {
		t.Helper()
		var req protocol.CallToolRequest
		req.Params.Name = name
		req.Params.Arguments = args
		result, err := c.CallTool(ctx, req)
		return err == nil && !result.IsError
	}
	if !call("create_note", map[string]interface{}{"path": "one", "summary": "test", "content": "body"}) {
		t.Fatal("create failed")
	}
	if !call("read_note", map[string]interface{}{"path": "one"}) {
		t.Fatal("read failed")
	}
	for _, name := range []string{"get_index", "list_notes"} {
		if call(name, map[string]interface{}{"path": "../"}) {
			t.Fatal("escaped", name)
		}
	}
	if call("delete_note", map[string]interface{}{"path": "."}) {
		t.Fatal("root deletion accepted")
	}
	if !call("rename_note", map[string]interface{}{"old": "one", "new": "two"}) {
		t.Fatal("rename failed")
	}
	if !call("delete_note", map[string]interface{}{"path": "two"}) {
		t.Fatal("delete failed")
	}
}

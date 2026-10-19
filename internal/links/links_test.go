package links

import (
	"reflect"
	"testing"
)

func TestCanonicalLinksIgnoreCodeAndExternal(t *testing.T) {
	body := []byte("[root](/root.md) [parent](../parent.md) [local](./local.md#section) [ext](https://example.com/no.md) `[[code]]` [[Real]]\n\n```\n[bad](bad.md) [[ignored]]\n```\n")
	got := Extract(body, "folder", true)
	want := []string{"Real", "folder/local", "parent", "root"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
	if got := Resolve("../../secret.md", "folder"); got != "" {
		t.Fatal(got)
	}
}

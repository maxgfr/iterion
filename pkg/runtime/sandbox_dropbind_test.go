package runtime

import (
	"reflect"
	"testing"
)

func TestMountIsHostBind(t *testing.T) {
	cases := []struct {
		mount string
		want  bool
	}{
		// explicit type=bind (the ~/.claude OAuth mount bots author for docker)
		{"type=bind,source=/home/u/.claude,target=/home/devbox/.claude,consistency=cached", true},
		{"source=/x,target=/y,type=bind,readonly", true},
		// no explicit type + a source ⇒ docker's bind default
		{"source=/x,target=/y", true},
		// cloud-supported mount types are never host binds
		{"type=pvc,source=myclaim,target=/data", false},
		{"type=configmap,source=cm,target=/etc/cfg", false},
		{"type=secret,source=sec,target=/run/secrets/x", false},
	}
	for _, c := range cases {
		if got := mountIsHostBind(c.mount); got != c.want {
			t.Errorf("mountIsHostBind(%q) = %v, want %v", c.mount, got, c.want)
		}
	}
}

func TestDropHostBindMounts(t *testing.T) {
	in := []string{
		"type=bind,source=/home/u/.claude,target=/home/devbox/.claude,consistency=cached",
		"type=pvc,source=claim,target=/data",
		"source=/host/bin,target=/usr/local/bin/iterion,type=bind,readonly",
		"type=secret,source=sec,target=/run/secrets/forge_token",
	}
	got := dropHostBindMounts(in, nil)
	want := []string{
		"type=pvc,source=claim,target=/data",
		"type=secret,source=sec,target=/run/secrets/forge_token",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("dropHostBindMounts kept %v, want %v", got, want)
	}
	// nil / empty passes through untouched.
	if out := dropHostBindMounts(nil, nil); out != nil {
		t.Errorf("dropHostBindMounts(nil) = %v, want nil", out)
	}
}

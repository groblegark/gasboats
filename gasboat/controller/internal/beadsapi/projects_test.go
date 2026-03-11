package beadsapi

import "testing"

func TestChannelMode_Concierge(t *testing.T) {
	p := ProjectInfo{
		ChannelModes: map[string]string{
			"C111": "concierge",
			"C222": "mention",
		},
	}
	if got := p.ChannelMode("C111"); got != "concierge" {
		t.Errorf("ChannelMode(C111) = %q, want %q", got, "concierge")
	}
	if got := p.ChannelMode("C222"); got != "mention" {
		t.Errorf("ChannelMode(C222) = %q, want %q", got, "mention")
	}
}

func TestChannelMode_DefaultMention(t *testing.T) {
	p := ProjectInfo{
		ChannelModes: map[string]string{"C111": "concierge"},
	}
	if got := p.ChannelMode("C999"); got != "mention" {
		t.Errorf("ChannelMode(C999) = %q, want %q", got, "mention")
	}
}

func TestChannelMode_NilMap(t *testing.T) {
	p := ProjectInfo{}
	if got := p.ChannelMode("C111"); got != "mention" {
		t.Errorf("ChannelMode(C111) = %q, want %q", got, "mention")
	}
}

func TestParseSlackChannels(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{"empty", "", nil},
		{"single", "C123", []string{"C123"}},
		{"multi", "C123,C456, C789", []string{"C123", "C456", "C789"}},
		{"trailing comma", "C123,", []string{"C123"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSlackChannels(tt.raw)
			if len(got) != len(tt.want) {
				t.Fatalf("parseSlackChannels(%q) = %v, want %v", tt.raw, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseSlackChannels(%q)[%d] = %q, want %q", tt.raw, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestHasChannel(t *testing.T) {
	p := ProjectInfo{SlackChannels: []string{"C111", "C222"}}
	if !p.HasChannel("C111") {
		t.Error("HasChannel(C111) = false, want true")
	}
	if p.HasChannel("C999") {
		t.Error("HasChannel(C999) = true, want false")
	}
}

func TestChannelRole(t *testing.T) {
	p := ProjectInfo{ChannelRoles: map[string]string{"C111": "ops"}}
	if got := p.ChannelRole("C111"); got != "ops" {
		t.Errorf("ChannelRole(C111) = %q, want %q", got, "ops")
	}
	if got := p.ChannelRole("C999"); got != "" {
		t.Errorf("ChannelRole(C999) = %q, want %q", got, "")
	}
}

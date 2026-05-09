package manager

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

func genValidIPv4(t *rapid.T) string {
	a := rapid.IntRange(0, 255).Draw(t, "a")
	b := rapid.IntRange(0, 255).Draw(t, "b")
	c := rapid.IntRange(0, 255).Draw(t, "c")
	d := rapid.IntRange(0, 255).Draw(t, "d")
	return fmt.Sprintf("%d.%d.%d.%d", a, b, c, d)
}

func genInvalidIP(t *rapid.T) string {
	candidates := []string{
		"not-an-ip",
		"256.0.0.1",
		"1.2.3",
		"1.2.3.4.5",
		"abc",
		"",
		"999.999.999.999",
		"1.2.3.-1",
		"::gggg",
		"hostname.example.com",
	}
	idx := rapid.IntRange(0, len(candidates)-1).Draw(t, "invalid_ip_idx")
	return candidates[idx]
}

func genIPSet(t *rapid.T) []string {
	count := rapid.IntRange(0, 5).Draw(t, "ip_count")
	seen := make(map[string]bool)
	var ips []string
	for i := 0; i < count; i++ {
		for attempt := 0; attempt < 20; attempt++ {
			ip := genValidIPv4(t)
			if !seen[ip] {
				seen[ip] = true
				ips = append(ips, ip)
				break
			}
		}
	}
	return ips
}


func genResolvConf(t *rapid.T) string {
	lineCount := rapid.IntRange(0, 10).Draw(t, "line_count")
	lineKinds := []string{"nameserver", "comment", "empty", "other"}

	var lines []string
	for i := 0; i < lineCount; i++ {
		kindIdx := rapid.IntRange(0, len(lineKinds)-1).Draw(t, fmt.Sprintf("line_kind_%d", i))
		kind := lineKinds[kindIdx]
		switch kind {
		case "nameserver":
			ip := genValidIPv4(t)
			lines = append(lines, "nameserver "+ip)
		case "comment":
			comment := rapid.StringMatching(`[a-zA-Z0-9 ._-]*`).Draw(t, fmt.Sprintf("comment_%d", i))
			lines = append(lines, "# "+comment)
		case "empty":
			lines = append(lines, "")
		case "other":
			directives := []string{
				"search example.com",
				"domain local",
				"options ndots:5",
				"options timeout:2",
				"sortlist 130.155.160.0/255.255.240.0",
			}
			idx := rapid.IntRange(0, len(directives)-1).Draw(t, fmt.Sprintf("directive_%d", i))
			lines = append(lines, directives[idx])
		}
	}

	if len(lines) == 0 {
		if rapid.Bool().Draw(t, "empty_with_newline") {
			return "\n"
		}
		return ""
	}

	content := strings.Join(lines, "\n")
	if rapid.Bool().Draw(t, "trailing_newline") {
		content += "\n"
	}
	return content
}

func TestParse_Table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    []line
		wantErr error
	}{
		{
			name:    "empty file",
			content: "",
			want:    []line{},
		},
		{
			name:    "single newline",
			content: "\n",
			want:    []line{},
		},
		{
			name:    "no trailing newline still parsed",
			content: "nameserver 1.1.1.1",
			want: []line{
				{kind: lineNameserverIP, raw: "nameserver 1.1.1.1", ip: "1.1.1.1"},
			},
		},
		{
			name:    "comment above nameserver",
			content: "# comment\nnameserver 8.8.8.8\n",
			want: []line{
				{kind: lineOther, raw: "# comment"},
				{kind: lineNameserverIP, raw: "nameserver 8.8.8.8", ip: "8.8.8.8"},
				{kind: lineOther, raw: ""},
			},
		},
		{
			name:    "invalid directive preserved",
			content: "branot namesspacer\n",
			want: []line{
				{kind: lineOther, raw: "branot namesspacer"},
				{kind: lineOther, raw: ""},
			},
		},
		{
			name:    "invalid nameserver ip treated as other",
			content: "nameserver not-an-ip\n",
			wantErr: ErrInvalidConfig,
		},
		{
			name:    "nameserver with inline comment treated as other",
			content: "nameserver 1.1.1.1 # cloudflare\n",
			wantErr: ErrInvalidConfig,
		},
		{
			name:    "mixed content preserved",
			content: "# c\noptions ndots:5\nnameserver 9.9.9.9\n\n",
			want: []line{
				{kind: lineOther, raw: "# c"},
				{kind: lineOther, raw: "options ndots:5"},
				{kind: lineNameserverIP, raw: "nameserver 9.9.9.9", ip: "9.9.9.9"},
				{kind: lineOther, raw: ""},
				{kind: lineOther, raw: ""},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parse(tt.content)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("parse(%q) expected error %v, got nil", tt.content, tt.wantErr)
				}
				if err != tt.wantErr {
					t.Fatalf("parse(%q) expected error %v, got %v", tt.content, tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse(%q) unexpected error: %v", tt.content, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parse(%q) mismatch:\nwant: %#v\ngot:  %#v", tt.content, tt.want, got)
			}
		})
	}
}

func TestParse_Table_WithGeneratedIPs(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name  string
		build func(t *rapid.T) (string, []line, error)
	}

	tests := []testCase{
		{
			name: "generated valid nameserver ip is parsed",
			build: func(t *rapid.T) (string, []line, error) {
				ip := genValidIPv4(t)
				content := "nameserver " + ip + "\n"
				want := []line{
					{kind: lineNameserverIP, raw: "nameserver " + ip, ip: ip},
					{kind: lineOther, raw: ""},
				}
				return content, want, nil
			},
		},
		{
			name: "generated invalid ip in nameserver returns error",
			build: func(t *rapid.T) (string, []line, error) {
				ip := genInvalidIP(t)
				content := "nameserver " + ip + "\n"
				return content, nil, ErrInvalidConfig
			},
		},
		{
			name: "generated ip set with comments and invalid directive still parses",
			build: func(t *rapid.T) (string, []line, error) {
				ips := genIPSet(t)
				invalidLine := "branot namesspacer"

				var contentLines []string
				var want []line

				contentLines = append(contentLines, "# comment before nameservers")
				want = append(want, line{kind: lineOther, raw: "# comment before nameservers"})

				contentLines = append(contentLines, "search example.com")
				want = append(want, line{kind: lineOther, raw: "search example.com"})

				contentLines = append(contentLines, invalidLine)
				want = append(want, line{kind: lineOther, raw: invalidLine})

				for _, ip := range ips {
					contentLines = append(contentLines, "nameserver "+ip)
					want = append(want, line{kind: lineNameserverIP, raw: "nameserver " + ip, ip: ip})
				}

				contentLines = append(contentLines, "")
				want = append(want, line{kind: lineOther, raw: ""})

				content := strings.Join(contentLines, "\n") + "\n"
				want = append(want, line{kind: lineOther, raw: ""})
				return content, want, nil
			},
		},
		{
			name: "no ip at all only comments and unknown directives",
			build: func(t *rapid.T) (string, []line, error) {
				_ = genIPSet(t)
				content := "# only comments\n# and garbage\nbranot namesspacer\n\n"
				want := []line{
					{kind: lineOther, raw: "# only comments"},
					{kind: lineOther, raw: "# and garbage"},
					{kind: lineOther, raw: "branot namesspacer"},
					{kind: lineOther, raw: ""},
					{kind: lineOther, raw: ""},
				}
				return content, want, nil
			},
		},
		{
			name: "nameserver without ip returns error",
			build: func(t *rapid.T) (string, []line, error) {
				content := "nameserver\n"
				return content, nil, ErrInvalidConfig
			},
		},
		{
			name: "nameserver with inline comment returns error",
			build: func(t *rapid.T) (string, []line, error) {
				ip := genValidIPv4(t)
				content := "nameserver " + ip + " # comment\n"
				return content, nil, ErrInvalidConfig
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rapid.Check(t, func(rt *rapid.T) {
				content, want, wantErr := tc.build(rt)
				got, err := parse(content)
				if wantErr != nil {
					if err == nil {
						t.Fatalf("parse(%q) expected error %v, got nil", content, wantErr)
					}
					if err != wantErr {
						t.Fatalf("parse(%q) expected error %v, got %v", content, wantErr, err)
					}
					return
				}
				if err != nil {
					t.Fatalf("parse(%q) unexpected error: %v", content, err)
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("parse(%q) mismatch:\nwant: %#v\ngot:  %#v", content, want, got)
				}
			})
		})
	}
}

func TestProp_ParseFormatRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		content := genResolvConf(t)
		parsed1, err := parse(content)
		if err != nil {
			t.Fatalf("parse(content) unexpected error: %v, content=%q", err, content)
		}
		formatted := format(parsed1)
		parsed2, err := parse(formatted)
		if err != nil {
			t.Fatalf("parse(formatted) unexpected error: %v, formatted=%q", err, formatted)
		}
		if !reflect.DeepEqual(parsed1, parsed2) {
			t.Fatalf("round-trip mismatch:\ncontent:   %q\nparsed1:   %v\nformatted: %q\nparsed2:   %v",
				content, parsed1, formatted, parsed2)
		}
	})
}

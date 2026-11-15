package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseEnvFile(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    map[string]string
		wantErr string
	}{
		{name: "empty", input: "", want: map[string]string{}},
		{
			name:  "plain pairs",
			input: "A=1\nB=two words\n",
			want:  map[string]string{"A": "1", "B": "two words"},
		},
		{
			name:  "comments and blanks",
			input: "# hash\n; semicolon\n\n  A = 1  \n",
			want:  map[string]string{"A": "1"},
		},
		{
			name:  "last duplicate wins",
			input: "A=1\nA=2\n",
			want:  map[string]string{"A": "2"},
		},
		{
			name:  "empty value",
			input: "A=\n",
			want:  map[string]string{"A": ""},
		},
		{
			name:  "value keeps equals and hash",
			input: "A=x=y#z\n",
			want:  map[string]string{"A": "x=y#z"},
		},
		{
			name:  "double quotes with escapes",
			input: `A="a \"b\" \\ c\nd"` + "\n",
			want:  map[string]string{"A": "a \"b\" \\ c\nd"},
		},
		{
			name:  "single quotes are literal",
			input: `A='a \n "b"'` + "\n",
			want:  map[string]string{"A": `a \n "b"`},
		},
		{
			name:  "no trailing newline and CRLF",
			input: "A=1\r\nB=2",
			want:  map[string]string{"A": "1", "B": "2"},
		},
		{name: "missing equals", input: "A\n", wantErr: "line 1: expected KEY=value"},
		{name: "bad name", input: "ok=1\n1A=2\n", wantErr: "line 2: invalid variable name"},
		{name: "empty name", input: "=1\n", wantErr: "invalid variable name"},
		{name: "unterminated double", input: "A=\"x\n", wantErr: "unterminated double quote"},
		{name: "unterminated single", input: "A='x\n", wantErr: "unterminated single quote"},
		{name: "unknown escape", input: `A="\q"` + "\n", wantErr: "unknown escape"},
		{name: "inner quote", input: `A="a"b"` + "\n", wantErr: "unescaped double quote"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseEnvFile(strings.NewReader(tt.input))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

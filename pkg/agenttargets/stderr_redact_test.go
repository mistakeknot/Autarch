package agenttargets

import (
	"strings"
	"testing"
)

// Stages and exact outputs follow the reviewed plan for mk-rkkao.2.
const redactStage = 5

type redactCase struct {
	input, want string
	greenFrom   int
}

var newShapeCases = []redactCase{
	{"api_key=1111&ok=yes", "api_key=[REDACTED]&ok=yes", 3},
	{"token=1111", "token=[REDACTED]", 3},
	{"'access_token'='1111'", "'access_token'=[REDACTED]", 3},
	{"\"token\" = \"11 11\"", "\"token\" = [REDACTED]", 3},
	{"token=\"11\\\"11\" ok", "token=[REDACTED] ok", 3},
	{"token=\"1111 ok", "token=[REDACTED]", 3},
	{"ANTHROPIC_API_KEY=1111", "ANTHROPIC_API_KEY=[REDACTED]", 3},
	{"GITHUB_TOKEN=1111", "GITHUB_TOKEN=[REDACTED]", 3},
	{"CLAUDE_CODE_OAUTH_TOKEN=1111", "CLAUDE_CODE_OAUTH_TOKEN=[REDACTED]", 3},
	{"DB_PASSWORD=1111", "DB_PASSWORD=[REDACTED]", 3},
	{"auth_token=1111", "auth_token=[REDACTED]", 3},
	{"private_key=1111", "private_key=[REDACTED]", 3},
	{"passwd=1111", "passwd=[REDACTED]", 3},
	{"(api_key=1111)", "(api_key=[REDACTED]", 3},
	{"[token=1111]", "[token=[REDACTED]", 3},
	{"--password=1111", "--password=[REDACTED]", 3},
	{"https://example.test/x?token=1111&a=b", "https://example.test/x?token=[REDACTED]&a=b", 3},
	{"access_token=1111: expired", "[REDACTED] expired", 0},
	{"token= 1111", "token= [REDACTED]", 3},
	{"CACHE_KEY=build-1", "CACHE_KEY=[REDACTED]", 3},
	{"token=\"11\n22\" ok", "token=[REDACTED]\n22\" ok", 3},
	{"token=\n1111", "token=\n1111", 0},
	{"PWD=/tmp/build", "PWD=/tmp/build", 0},
	{"max_tokens=4096", "max_tokens=4096", 0},
	{"token_count=12", "token_count=12", 0},
	{"{\"password\":\"11 11\"}", "{\"password\":[REDACTED]}", 3},
	{"\"password\":\n\"1111\"", "\"password\":\n[REDACTED]", 3},
	{"{\"password\": \"11\n22\"}", "{\"password\": [REDACTED]}", 3},
	{"{\"token\": \"11\n22\"}", "{[REDACTED]}", 0},
	{"\"token\"\n: \"1111\"", "[REDACTED]", 0},
	{"{\"accessToken\":\"1111\"}", "{\"accessToken\":[REDACTED]}", 3},
	{"{\"secretKey\":\"1111\"}", "{\"secretKey\":[REDACTED]}", 3},
	{"{\"Authorization\":\"1111\"}", "{\"Authorization\":[REDACTED]}", 3},
	{"{\\\"token\\\":\\\"11\\\\\\\"11\\\"}", "{\\\"token\\\":[REDACTED]}", 3},
	{"{\"cache_key\": 1111}", "{\"cache_key\": [REDACTED]}", 3},
	{"Authorization: Basic 11 11", "[REDACTED]", 0},
	{"Proxy-Authorization: Bot 11", "Proxy-[REDACTED]", 0},
	{"prefixAuthorization: Bot 11", "prefix[REDACTED]", 0},
	{"x-api-key: 1111 ok", "[REDACTED]", 0},
	{"Cookie: session=11; b=22", "Cookie: [REDACTED]", 3},
	{"Set-Cookie: session=11", "Set-Cookie: [REDACTED]", 3},
	{"x-auth-token: 1111 ok", "x-auth-token: [REDACTED] ok", 3},
	{"api-key: 1111 ok", "api-key: [REDACTED] ok", 3},
	{"token: \"11 11\" ok", "token: [REDACTED] ok", 3},
	{"token: \"11\n22\" ok", "token: [REDACTED]\n22\" ok", 3},
	{"token: 1111", "token: [REDACTED]", 3},
	{"Error: token: expired", "Error: token: [REDACTED]", 3},
	{"secret: not found in vault", "secret: [REDACTED]", 3},
	{"/example/.config/api_key:42: syntax error", "/example/.config/api_key:[REDACTED] syntax error", 3},
	{"password:\n1111", "password:\n1111", 0},
	{"Authorization:\n1111", "[REDACTED]\n1111", 0},
	{"key: unknown field", "key: unknown field", 0},
	{"server.key", "server.key", 0},
	{"sort_key: asc", "sort_key: asc", 0},
	{"keyboard: unavailable", "keyboard: unavailable", 0},
	{"secretary: unavailable", "secretary: unavailable", 0},
	{"max_tokens: 4096", "max_tokens: 4096", 0},
	{"token count exceeded", "token count exceeded", 0},
	{"Error: quota exceeded", "Error: quota exceeded", 0},
	{"disk-full; task-runner", "disk-full; task-runner", 0},
	{"{\"message\":\"invalid x-api-key\"}", "{\"message\":\"invalid x-api-key\"}", 0},
	{"--password \"11 11\" --verbose", "--password [REDACTED] --verbose", 3},
	{"--api-key 1111", "--api-key [REDACTED]", 3},
	{"--token 1111", "--token [REDACTED]", 3},
	{"--client-secret 1111", "--client-secret [REDACTED]", 3},
	{"--password \"11\n22\" x", "--password [REDACTED]\n22\" x", 3},
	{"--token-file /tmp/x", "--token-file /tmp/x", 0},
	{"--tokens 5", "--tokens 5", 0},
	{"--password\n1111", "--password\n1111", 0},
	{"https://xxx:1111@example.test/error", "https://xxx:[REDACTED]@example.test/error", 3},
	{"https://xxx:11@1@example.test/", "https://xxx:[REDACTED]@example.test/", 3},
	{"https://xxx:11/1@example.test/", "https://xxx:[REDACTED]@example.test/", 3},
	{"https://xxx:11/@1@example.test/", "https://xxx:[REDACTED]@example.test/", 3},
	{"https://1111@example.test/", "https://[REDACTED]@example.test/", 3},
	{"https://1111@example.test:443/", "https://[REDACTED]@example.test:443/", 3},
	{"HTTPS://xxx:1111@example.test/", "HTTPS://xxx:[REDACTED]@example.test/", 3},
	{"https://example.test:8080/a@b", "https://example.test:[REDACTED]@b", 3},
	{"https://u:1@host.example.test/pkg@v1", "https://u:[REDACTED]@v1", 3},
	{"https://registry.example.test/@scope%2fpkg", "https://registry.example.test/@scope%2fpkg", 0},
	{"https://example.test/pkg@v1.2.3: not found", "https://example.test/pkg@v1.2.3: not found", 0},
	{"git@example.test:org/repo.git", "git@example.test:org/repo.git", 0},
	{"token=abc\"secret\": \"11 11\"", "token=[REDACTED] [REDACTED]", 3},
	{"[stderr withheld: 4096 bytes with no complete line]", "[stderr withheld: 4096 bytes with no complete line]", 0},
}

func checkRedactStage(t *testing.T, c redactCase, streamed bool) {
	t.Helper()
	data := []byte(c.input)
	if streamed {
		var tail stderrTail
		tail.Write(data)
		data = tail.data
	}
	checkMainSuperset(t, data)
	got := sanitizedStderr(data)
	if (got == c.want) != (redactStage >= c.greenFrom) {
		t.Fatalf("stage %d greenFrom %d input %q: got %q want %q", redactStage, c.greenFrom, c.input, got, c.want)
	}
}
func TestRedactNewShapes(t *testing.T) {
	for _, c := range newShapeCases {
		t.Run(c.input, func(t *testing.T) { checkRedactStage(t, c, false) })
	}
}
func TestLegacyMaskGuards(t *testing.T) {
	for _, input := range []string{"Authorization: abc\rdef", "sk-abc\rdef", "Bearer abc\rdef", "access_token=abc\rdef", "x-api-key: abc\rdef ghi", "Autho\rrization: 1111", "Bea\rrer 1111", "s\rk-1111", "access_to\rken=1111", "x-api-\rkey: 1111"} {
		checkRedactStage(t, redactCase{input, "[REDACTED]", 0}, false)
	}
	for _, prefix := range []string{"#", ";", "/", ".", ":", "=", "|", "<", "-", "token: expired;"} {
		input := prefix + "access_token=1111"
		// This guard checks legacy coverage, not preservation of prose that a
		// new WEAK colon match deliberately hides from stage 3.
		checkMainSuperset(t, []byte(input))
		if strings.Contains(sanitizedStderr([]byte(input)), "1111") {
			t.Fatal("legacy value survived")
		}
	}
	for _, prefix := range []string{"Authorization: ", `{"token":"`} {
		checkRedactStage(t, redactCase{prefix + strings.Repeat("x", 5000) + "\rxxxx yyyy\nok", "ok", 0}, true)
	}
	for _, input := range []string{"Authorization: " + strings.Repeat("Q", 6000), "Cookie: " + strings.Repeat("Q", 6000), "x Bearer " + strings.Repeat("Q", 5000), "Bea\x1b[mrer " + strings.Repeat("Q", 1100) + "\r" + strings.Repeat("Q", 5000), `"token":` + strings.Repeat(" ", 1100) + "\n\"" + strings.Repeat("Q", 5000) + `"`} {
		checkRedactStage(t, redactCase{input, "[stderr withheld: 4096 bytes with no complete line]", 0}, true)
	}
}
func TestRegionAMask(t *testing.T) {
	checkRedactStage(t, redactCase{strings.Repeat("x", 5000) + "\x1b]0;t\nsk-1111\x07 ok", "[REDACTED] ok", 0}, true)
}
func TestCrossBoundaryMask(t *testing.T) {
	checkRedactStage(t, redactCase{strings.Repeat("x", 5000) + `{"token": "aaa` + "\nQQQ\" more", "[REDACTED] more", 2}, true)
}
func TestEveryCandidate(t *testing.T) {
	checkRedactStage(t, redactCase{`token=abc"secret": "11 11"`, "token=[REDACTED] [REDACTED]", 3}, false)
}
func TestFrameSelection(t *testing.T) {
	checkRedactStage(t, redactCase{"Working...\rWorking...\rError: quota exceeded", "Error: quota exceeded", 5}, false)
	for _, c := range []redactCase{
		{"Working\r\x1b]0;x\ry\x07Error: quota exceeded", "Error: quota exceeded", 5},
		{"keep\x1b]0;x\ry\x07 useful", "keep useful", 0},
		{"old\rnew\r\n", "new\n", 5},
		{"old\rnew\nfirst\rsecond", "new\nsecond", 5},
		{"old\r", "", 5},
		{"Bearer \xc3\r\xa9", "[REDACTED]", 0},
	} {
		checkRedactStage(t, c, false)
	}
}
func TestStraddlingRunRedacted(t *testing.T) {
	for _, c := range []redactCase{{"Error: Authorization: abc\rdef ok", "[REDACTED]", 5}, {"Working \rsk-11\r22 ok", "[REDACTED] ok", 5}} {
		checkRedactStage(t, c, false)
	}
}

func legacyGuardInputs() []string {
	inputs := []string{"Authorization: abc\rdef", "sk-abc\rdef", "Bearer abc\rdef", "access_token=abc\rdef", "x-api-key: abc\rdef ghi"}
	for _, name := range []string{"Authorization", "sk-", "Bearer", "access_token=", "x-api-key"} {
		for i := 0; i < len(name); i++ {
			inputs = append(inputs, name[:i]+"\r"+name[i:]+" 1111")
		}
	}
	for _, prefix := range []string{"#", ";", "/", ".", ":", "=", "|", "<", "-", "token: expired;"} {
		inputs = append(inputs, prefix+"access_token=1111")
	}
	inputs = append(inputs, `{"token": "11`+"\n"+`11"}`)
	for _, prefix := range []string{"Cookie: ", "token=\"", "okie:", "Authorization: ", `{"token":"`} {
		inputs = append(inputs, prefix+strings.Repeat("Q", 5500)+"\n")
	}
	for _, prefix := range []string{"Authorization: Bearer ", `{"access_token":"`, "sk-"} {
		inputs = append(inputs, prefix+strings.Repeat("Q", 300)+strings.Repeat("\x1b[2K\x1b[1G", 500))
	}
	return inputs
}

func TestNormalizeIndexMap(t *testing.T) {
	for _, input := range []string{"\xc3\x1b[0m\xa9", "Bearer \xef\xbf\xbd11 ok", "Bearer \xff11 ok", "\x1b]0;t\nx\x07y", "\x1b[0m\x1b[0msk-1111 ok"} {
		normalized, src, _ := referenceRegion(t, []byte(input), 0)
		got := normalizeStderr([]byte(input), 0)
		if string(got.text) != normalized || len(got.source) != len(src) {
			t.Fatalf("normalization %q", input)
		}
		for i, p := range src {
			if got.source[i] != int32(p) {
				t.Fatalf("source %q at %d", input, i)
			}
		}
		if sanitizedStderr([]byte(input)) != frozenStderr([]byte(input)) {
			t.Fatalf("normalization output %q", input)
		}
	}
}

func TestPartialMatches(t *testing.T) {
	// Main does not cover bare long-s: RE2's leading word boundary is ASCII.
	// The prefixed row below exercises Unicode folding without changing rules.
	if got := sanitizedStderr([]byte("ſk-11")); got != "ſk-11" || got != frozenStderr([]byte("ſk-11")) {
		t.Fatalf("bare long-s changed: %q", got)
	}

	if got := sanitizedStderr([]byte("xſk-11")); got != "x[REDACTED]" {
		t.Fatalf("Unicode fold and visible prefix: %q", got)
	}
	for _, input := range []string{"sk-11é77", "sk-11\xc3\x1b[0m\xa977", "sk-11\xff77", "ghp_11-77", "Bearer 11\t77", "access_token=11&77", `"token": "11\\"77`, "xſk-11"} {
		checkMainSuperset(t, []byte(input))
		got := sanitizedStderr([]byte(input))
		if strings.Contains(got, "1") || strings.Count(got, "7") > strings.Count(frozenStderr([]byte(input)), "7") {
			t.Fatalf("partial match %q: %q", input, got)
		}
	}
}
func TestAcceptedOverRedaction(t *testing.T) {
	for _, input := range []string{"Error: token: expired", "secret: not found in vault", "/example/.config/api_key:42: syntax error", "CACHE_KEY=build-1", "https://example.test:8080/a@b", "https://u:1@host.example.test/pkg@v1", "(api_key=1111)", "[token=1111]"} {
		for _, c := range newShapeCases {
			if c.input == input {
				checkRedactStage(t, c, false)
			}
		}
	}
	checkRedactStage(t, redactCase{strings.Repeat("x", 5000) + `{"password":"1111` + "\nuseful", "[REDACTED]", 3}, true)
}

// Historical test fixtures are ported under the compatibility decisions for
// mk-rkkao.2; implementation is independent of the earlier attempt.
func TestPortedCompatibility(t *testing.T) {

	for name, cases := range portedDirectCases {
		t.Run(name+"/direct", func(t *testing.T) {
			for _, c := range cases {
				checkRedactStage(t, c, false)
			}
		})
	}
	// Exact replacement for the old spinner expectation at stages 3–4 and 5.
	want := "token=[REDACTED]"
	if redactStage >= 5 {
		want = "[REDACTED]"
	}
	checkRedactStage(t, redactCase{"token=xxx\rdone", want, 3}, false)
	const notice = "[stderr withheld: 4096 bytes with no complete line]"
	for _, prefix := range []string{"token=", "Cookie: ", `secret = "`, "okie: "} {
		for _, ending := range []string{"", "\n", "\r\n"} {
			checkRedactStage(t, redactCase{prefix + strings.Repeat("x ", 3000) + "end" + ending, notice, 0}, true)
		}
	}
	for _, pair := range []string{"Bearer 1111", "token: 1111", "x-api-key: 1111", `"token": "1111"`, "api_key = 1111", "Authorization: Basic 1111", `secret = "11 11"`, "https://xxx:1111@example.test/"} {
		for offset := 0; offset < len(pair); offset++ {
			t.Run("TestReviewCredentialCuts/streamed", func(t *testing.T) {
				checkRedactStage(t, redactCase{strings.Repeat("x", 5000) + pair + strings.Repeat(" ", 512-len(pair)+offset) + "end", notice, 0}, true)
			})
			t.Run("TestRoundTwoRawCuts/streamed", func(t *testing.T) {
				checkRedactStage(t, redactCase{strings.Repeat("x", 5000) + pair[offset:] + strings.Repeat("\x1b[0m", 1024), notice, 0}, true)
			})
		}
	}
}

var portedDirectCases = map[string][]redactCase{
	"TestCredentialAssignments":                {{`secret = "xxxx xxxx"`, `secret = [REDACTED]`, 3}},
	"TestReviewOrdinaryErrors":                 {{"open /etc/ssl/server.key: no such file or directory", "open /etc/ssl/server.key: no such file or directory", 0}, {"error parsing config.yaml: key: unknown field", "error parsing config.yaml: key: unknown field", 0}, {"monkey: banana / hotkey=ctrl+c", "monkey: banana / hotkey=ctrl+c", 0}},
	"TestReviewQuotingAndFrames":               {{`{\"access_token\":\"1111\"}`, `{\"access_token\":[REDACTED]}`, 3}, {`{\"token\": \"1111\"}`, `{\"token\": [REDACTED]}`, 3}},
	"TestRoundTwoAuthorization":                {{"Authorization: token 1111", "[REDACTED]", 0}, {"Authorization: Bot 1111", "[REDACTED]", 0}, {`Authorization: Digest username="xxx", response="1111"`, "[REDACTED]", 0}, {"Authorization: AWS4-HMAC-SHA256 Credential=xxx, Signature=1111", "[REDACTED]", 0}, {"prefixAuthorization: Bot 1111", "prefix[REDACTED]", 0}},
	"TestRoundTwoAdditionalNamesAndExceptions": {{"token: expiredxxxxxxxxxxxx", "token: [REDACTED]", 3}, {"token=expiredxxxxxxxxxxxx", "token=[REDACTED]", 3}, {"https://example.test/x?token=1111", "https://example.test/x?token=[REDACTED]", 3}},
	"TestRoundThreeAssignmentSeparator":        {{"access_token=1111: expired", "[REDACTED] expired", 0}, {"token=1111: expired", "token=[REDACTED] expired", 3}, {"password=1111:unknown field", "password=[REDACTED] field", 3}},
	"TestRoundThreeURLPassword":                {{"https://xxx:xx@1111@example.test/", "https://xxx:[REDACTED]@example.test/", 3}, {"https://xxx:xx/1111@example.test/", "https://xxx:[REDACTED]@example.test/", 3}, {"https://xxx:xx/@1111@example.test/", "https://xxx:[REDACTED]@example.test/", 3}},
	"TestRoundTwoCRLF":                         {{"Error: quota exceeded\r\nsecond line\r\n", "Error: quota exceeded\nsecond line\n", 0}, {"< HTTP/1.1 401 Unauthorized\r\n< x: y\r\n", "< HTTP/1.1 401 Unauthorized\n< x: y\n", 0}, {"Working...\rError: quota exceeded\r\n", "Error: quota exceeded\n", 5}},
}

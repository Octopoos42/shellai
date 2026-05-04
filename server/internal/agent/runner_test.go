package agent

import "testing"

func TestParsePlannerResponse_JSON(t *testing.T) {
	raw := `{"needs_tools": false, "plan_description": ""}`
	plan, err := parsePlannerResponse(raw)
	if err != nil {
		t.Fatalf("parsePlannerResponse() error = %v", err)
	}
	if plan.NeedsTools {
		t.Fatalf("expected needs_tools=false, got true")
	}
}

func TestParsePlannerResponse_BracketToolCallFallback(t *testing.T) {
	raw := `[Tool Call: shell]
Args: map[command:echo hi]
Explanation: check mailbox state first.`

	plan, err := parsePlannerResponse(raw)
	if err != nil {
		t.Fatalf("parsePlannerResponse() error = %v", err)
	}
	if !plan.NeedsTools {
		t.Fatalf("expected needs_tools=true, got false")
	}
	if plan.PlanDescription != "check mailbox state first." {
		t.Fatalf("unexpected plan_description: %q", plan.PlanDescription)
	}
}

func TestParseToolCallDecision_BracketToolCallMultilineCommand(t *testing.T) {
	raw := `[Tool Call: shell]
Args: map[command:echo 'import os
print(123)' > tmp.py && python3 tmp.py]
Explanation: run a small python check.`

	decision, err := parseToolCallDecision(raw)
	if err != nil {
		t.Fatalf("parseToolCallDecision() error = %v", err)
	}
	if decision.Tool != "shell" {
		t.Fatalf("expected tool=shell, got %q", decision.Tool)
	}
	cmd, _ := decision.Args["command"].(string)
	want := "echo 'import os\nprint(123)' > tmp.py && python3 tmp.py"
	if cmd != want {
		t.Fatalf("unexpected command:\nwant: %q\ngot : %q", want, cmd)
	}
	if decision.Explanation != "run a small python check." {
		t.Fatalf("unexpected explanation: %q", decision.Explanation)
	}
}

func TestExtractFirstJSONObject(t *testing.T) {
	raw := "intro text\n{\"needs_tools\":true,\"plan_description\":\"x\"}\ntrailing"
	obj, ok := extractFirstJSONObject(raw)
	if !ok {
		t.Fatalf("expected to find JSON object")
	}
	want := `{"needs_tools":true,"plan_description":"x"}`
	if obj != want {
		t.Fatalf("unexpected object: %q", obj)
	}
}

func TestParseToolCallDecision_RelaxedMalformedJSONMultilineCommand(t *testing.T) {
	raw := "{\n \"tool\": \"shell\",\n \"args\": {\n \"command\": \"echo 'import imaplib\nimport email\nprint(1)' > tmp.py && python3 tmp.py\"\n },\n \"explanation\": \"Checking your mailbox for unread emails using IMAP.\"\n}"

	decision, err := parseToolCallDecision(raw)
	if err != nil {
		t.Fatalf("parseToolCallDecision() error = %v", err)
	}

	if decision.Tool != "shell" {
		t.Fatalf("expected tool=shell, got %q", decision.Tool)
	}

	cmd, _ := decision.Args["command"].(string)
	wantCmd := "echo 'import imaplib\nimport email\nprint(1)' > tmp.py && python3 tmp.py"
	if cmd != wantCmd {
		t.Fatalf("unexpected command:\nwant: %q\ngot : %q", wantCmd, cmd)
	}

	wantExpl := "Checking your mailbox for unread emails using IMAP."
	if decision.Explanation != wantExpl {
		t.Fatalf("unexpected explanation:\nwant: %q\ngot : %q", wantExpl, decision.Explanation)
	}
}

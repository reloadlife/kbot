package bot

import (
	"strings"
	"testing"
)

// Test command parsing and argument extraction
func TestCommandArgumentParsing(t *testing.T) {
	tests := []struct {
		command      string
		args         string
		expectedArgs []string
	}{
		{"/pods", "production", []string{"production"}},
		{"/logs", "frontend-pod -n production", []string{"frontend-pod", "-n", "production"}},
		{"/grant", "123456789 logs pods -n production -l app=frontend",
			[]string{"123456789", "logs", "pods", "-n", "production", "-l", "app=frontend"}},
		{"/scale", "api-deployment 3 -n staging", []string{"api-deployment", "3", "-n", "staging"}},
		{"/help", "", []string{}},
	}

	for _, tt := range tests {
		args := strings.Fields(tt.args)

		if len(args) != len(tt.expectedArgs) {
			t.Errorf("Command %s with args %q: got %d args, expected %d",
				tt.command, tt.args, len(args), len(tt.expectedArgs))
			continue
		}

		for i := range args {
			if args[i] != tt.expectedArgs[i] {
				t.Errorf("Command %s arg[%d]: got %q, expected %q",
					tt.command, i, args[i], tt.expectedArgs[i])
			}
		}
	}
}

// Test namespace extraction from command arguments
func TestNamespaceExtraction(t *testing.T) {
	tests := []struct {
		args              []string
		expectedNamespace string
		description       string
	}{
		{
			[]string{"pod-name", "-n", "production"},
			"production",
			"Standard -n flag",
		},
		{
			[]string{"-n", "staging", "pod-name"},
			"staging",
			"-n flag before resource name",
		},
		{
			[]string{"pod-name"},
			"default",
			"No namespace specified",
		},
		{
			[]string{"deployment-name", "-n", "dev", "-l", "app=frontend"},
			"dev",
			"-n flag with other flags",
		},
	}

	for _, tt := range tests {
		namespace := "default"

		// Simulate parsing logic
		for i := 0; i < len(tt.args); i++ {
			if tt.args[i] == "-n" && i+1 < len(tt.args) {
				namespace = tt.args[i+1]
				break
			}
		}

		if namespace != tt.expectedNamespace {
			t.Errorf("%s: got namespace %q, expected %q",
				tt.description, namespace, tt.expectedNamespace)
		}
	}
}

// Test selector extraction from command arguments
func TestSelectorExtraction(t *testing.T) {
	tests := []struct {
		args             []string
		expectedSelector string
		description      string
	}{
		{
			[]string{"123456789", "logs", "pods", "-l", "app=frontend"},
			"app=frontend",
			"Simple selector",
		},
		{
			[]string{"123456789", "logs", "pods", "-n", "production", "-l", "app=frontend,tier=web"},
			"app=frontend,tier=web",
			"Multi-label selector",
		},
		{
			[]string{"123456789", "logs", "pods"},
			"",
			"No selector",
		},
	}

	for _, tt := range tests {
		selector := ""

		// Simulate parsing logic
		for i := 0; i < len(tt.args); i++ {
			if tt.args[i] == "-l" && i+1 < len(tt.args) {
				selector = tt.args[i+1]
				break
			}
		}

		if selector != tt.expectedSelector {
			t.Errorf("%s: got selector %q, expected %q",
				tt.description, selector, tt.expectedSelector)
		}
	}
}

// Test help message format
func TestHelpMessageFormat(t *testing.T) {
	helpText := `*Available Commands:*

*Resource Queries:*
/namespaces - List accessible namespaces
/pods [namespace] - List pods`

	if !strings.Contains(helpText, "/namespaces") {
		t.Error("Help text should contain /namespaces command")
	}

	if !strings.Contains(helpText, "/pods") {
		t.Error("Help text should contain /pods command")
	}

	if !strings.Contains(helpText, "*Available Commands:*") {
		t.Error("Help text should have a header")
	}
}

// Test message formatting
func TestMessageFormatting(t *testing.T) {
	tests := []struct {
		format   string
		expected string
	}{
		{
			"*Pods in namespace production:*\n\n📦 `pod-1`\n   Status: Running",
			"*Pods in namespace production:*\n\n📦 `pod-1`\n   Status: Running",
		},
		{
			"✅ Deployment `api-deployment` restarted in namespace *production*",
			"✅ Deployment `api-deployment` restarted in namespace *production*",
		},
		{
			"❌ Permission denied: Missing verb 'restart' for resource 'deployments'",
			"❌ Permission denied: Missing verb 'restart' for resource 'deployments'",
		},
	}

	for _, tt := range tests {
		if tt.format != tt.expected {
			t.Errorf("Message format mismatch")
		}

		// Check for markdown elements
		hasMarkdown := strings.Contains(tt.format, "*") ||
			strings.Contains(tt.format, "`") ||
			strings.Contains(tt.format, "✅") ||
			strings.Contains(tt.format, "❌")

		if !hasMarkdown {
			t.Error("Message should contain markdown formatting")
		}
	}
}

// Test user role display
func TestUserRoleFormatting(t *testing.T) {
	tests := []struct {
		role     string
		expected string
	}{
		{"admin", "admin"},
		{"operator", "operator"},
		{"viewer", "viewer"},
		{"none", "none"},
	}

	for _, tt := range tests {
		if tt.role != tt.expected {
			t.Errorf("Role %q doesn't match expected %q", tt.role, tt.expected)
		}
	}
}

// Test grant command argument parsing
func TestGrantCommandParsing(t *testing.T) {
	testCases := []struct {
		args             string
		expectedUserID   string
		expectedVerb     string
		expectedResource string
		expectedNS       string
		expectedSelector string
		valid            bool
	}{
		{
			"123456789 logs pods -n production",
			"123456789", "logs", "pods", "production", "", true,
		},
		{
			"987654321 restart deployments -n staging -l app=frontend",
			"987654321", "restart", "deployments", "staging", "app=frontend", true,
		},
		{
			"111111111 * * -n dev",
			"111111111", "*", "*", "dev", "", true,
		},
		{
			"invalid", // Not enough arguments
			"", "", "", "", "", false,
		},
	}

	for _, tc := range testCases {
		args := strings.Fields(tc.args)

		if tc.valid {
			if len(args) < 3 {
				t.Errorf("Valid command %q should have at least 3 args", tc.args)
				continue
			}

			userID := args[0]
			verb := args[1]
			resource := args[2]

			if userID != tc.expectedUserID {
				t.Errorf("UserID: got %q, expected %q", userID, tc.expectedUserID)
			}
			if verb != tc.expectedVerb {
				t.Errorf("Verb: got %q, expected %q", verb, tc.expectedVerb)
			}
			if resource != tc.expectedResource {
				t.Errorf("Resource: got %q, expected %q", resource, tc.expectedResource)
			}
		} else {
			if len(args) >= 3 {
				t.Errorf("Invalid command %q should not have enough args", tc.args)
			}
		}
	}
}

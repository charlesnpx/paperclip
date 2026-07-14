package policy

import (
	"strings"
	"testing"
)

func TestScannerDeniesAndRedactsSecretLikeValues(t *testing.T) {
	scanner := DefaultScanner()
	secret := "token=ghp_abcdefghijklmnopqrstuvwxyz123456"
	err := scanner.Scan("observed", secret)
	if err == nil {
		t.Fatal("expected denial")
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "ghp_abcdefghijklmnopqrstuvwxyz123456") {
		t.Fatalf("error leaked rejected content: %v", err)
	}
	if !IsDenied(err) {
		t.Fatalf("IsDenied returned false")
	}
}

func TestScannerAllowsOrdinaryOperationalText(t *testing.T) {
	scanner := DefaultScanner()
	if err := scanner.Scan("observed", "the harness dropped stderr before tests compiled"); err != nil {
		t.Fatalf("unexpected denial: %v", err)
	}
}

func TestScannerDeniesCredentialBearingURLs(t *testing.T) {
	scanner := DefaultScanner()
	value := "fetch failed for https://build:SuperSecret123@registry.example/packages/tool"
	err := scanner.Scan("observed", value)
	if err == nil {
		t.Fatal("expected denial")
	}
	if strings.Contains(err.Error(), "SuperSecret123") || strings.Contains(err.Error(), "registry.example") {
		t.Fatalf("error leaked url content: %v", err)
	}
}

func TestScannerDeniesCredentialQueryURLs(t *testing.T) {
	scanner := DefaultScanner()
	values := []string{
		"https://x.invalid/p?safe=1;credentials%5BaccessToken%5D=opaque-credential-value",
		"https://example.invalid/%ZZ?access%5Ftoken=opaque-credential-value",
		"https://registry.example/pkg?access_token=SuperSecret123",
		"https://registry.example/pkg?accessToken=SuperSecret123",
		"https://registry.example/pkg?access_token[]=SuperSecret123",
		"https://registry.example/pkg?accessToken[0]=SuperSecret123",
		"https://registry.example/pkg?auth[token]=SuperSecret123",
		"https://registry.example/pkg?credentials[0][accessToken]=SuperSecret123",
		"https://registry.example/pkg?credentials[clientSecret]=SuperSecret123",
		"https://registry.example/pkg?headers[authorization]=SuperSecret123",
		"https://registry.example/pkg?state[session]=SuperSecret123",
		"https://registry.example/pkg?password=SuperSecret123",
		"https://registry.example/pkg?authToken=SuperSecret123",
		"https://registry.example/pkg?clientSecret=SuperSecret123",
		"https://registry.example/pkg?refreshToken=SuperSecret123",
		"https://storage.example/blob?sig=SuperSecret123",
		"https://s3.example/object?X-Amz-Signature=SuperSecret123",
		"https://service.example/api?authorization=Bearer%20SuperSecret123456",
		"https://service.example/api?auth=SuperSecret123",
		"https://service.example/api?session=SuperSecret123",
	}
	for _, value := range values {
		err := scanner.Scan("observed", value)
		if err == nil {
			t.Fatalf("expected denial for %s", value)
		}
		if strings.Contains(err.Error(), "SuperSecret123") || strings.Contains(err.Error(), "registry.example") {
			t.Fatalf("error leaked url content: %v", err)
		}
	}
}

func TestPersistableScanUsesURLPolicy(t *testing.T) {
	scanner := DefaultScanner()
	body := []byte(`{"payload":{"observed":"https://service.example/api?authorization=Bearer%20SuperSecret123456"}}`)
	err := scanner.ScanPersistable("persistable_event", body)
	if err == nil {
		t.Fatal("expected denial")
	}
	if strings.Contains(err.Error(), "SuperSecret123") || strings.Contains(err.Error(), "service.example") {
		t.Fatalf("error leaked url content: %v", err)
	}
}

func TestScannerDeniesAuthAndSessionAssignments(t *testing.T) {
	scanner := DefaultScanner()
	values := []string{
		"authorization=Basic dXNlcjpwYXNz",
		`{"authorization":"Basic dXNlcjpwYXNz"}`,
		`password=" CorrectHorseBatteryStaple"`,
		`"authorization":" Basic dXNlcjpwYXNz"`,
		`{\"accessToken\":\"secret\"}`,
		"auth=opaque-credential-value",
		"session=opaque-session-value",
		"accessToken=opaque-credential-value",
		`{"accessToken":"opaque-credential-value"}`,
		`{"credentials":{"accessToken":"opaque-credential-value"}}`,
		"authToken=opaque-credential-value",
		"clientSecret=opaque-credential-value",
		"refreshToken=opaque-credential-value",
		`password="CorrectHorseBatteryStaple"`,
		"passwd=CorrectHorseBatteryStaple",
		"pwd=CorrectHorseBatteryStaple",
	}
	for _, value := range values {
		err := scanner.Scan("observed", value)
		if err == nil {
			t.Fatalf("expected denial for %q", value)
		}
		if strings.Contains(err.Error(), "dXNlcjpwYXNz") || strings.Contains(err.Error(), "opaque") {
			t.Fatalf("error leaked assignment content: %v", err)
		}
		if err := scanner.ScanPersistable("persistable_event", []byte(value)); err == nil {
			t.Fatalf("expected persistable denial for %q", value)
		}
	}
}

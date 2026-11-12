package provider

import "testing"

func TestIngestionKeyIsSensitive(t *testing.T) {
	if !ingestionKeyResourceSchema().Attributes["key"].IsSensitive() {
		t.Error("key isn't Sensitive; it would print in plans, terraform show and CI logs")
	}
}

package provider

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/honeybadger-io/api-go/apiv3"
)

const accPrefix = "tf-acc-"

func TestMain(m *testing.M) {
	resource.TestMain(m)
}

func accClient() *apiv3.Client {
	return apiv3.NewClient().
		WithBaseURL(os.Getenv("HONEYBADGER_API_URL")).
		WithBearerToken(os.Getenv("HONEYBADGER_API_TOKEN"))
}

// testAccPreCheck needs the local stack's URL and Kevin's token, and enough
// rate-limit budget for a full run (spec 2b, section 5.3).
func testAccPreCheck(t *testing.T) {
	t.Helper()
	for _, v := range []string{"HONEYBADGER_API_URL", "HONEYBADGER_API_TOKEN"} {
		if os.Getenv(v) == "" {
			t.Fatalf("%s must be set for acceptance tests", v)
		}
	}
	c := accClient()
	if _, err := c.Tokens.Get(context.Background()); err != nil {
		t.Fatalf("checking the token: %v", err)
	}
	if rl := c.LastRateLimit(); rl != nil && rl.Remaining < 300 {
		t.Fatalf("only %d API requests left this hour; raise api_hits_per_hour for the token's account (CONTRIBUTING.md)", rl.Remaining)
	}
}

func init() {
	resource.AddTestSweepers("honeybadger_project", &resource.Sweeper{
		Name: "honeybadger_project",
		F: func(string) error {
			ctx := context.Background()
			c := accClient()
			projects, err := c.Projects.ListAll(ctx)
			if err != nil {
				return err
			}
			for _, p := range projects {
				if strings.HasPrefix(p.Name, accPrefix) {
					if err := c.Projects.Delete(ctx, p.Id); err != nil && !errors.Is(err, apiv3.ErrNotFound) {
						return fmt.Errorf("deleting %s: %w", p.Name, err)
					}
				}
			}
			return nil
		},
	})
}

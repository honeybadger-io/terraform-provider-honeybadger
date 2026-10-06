// Command specgen generates each resource's schema and model from the OpenAPI
// bundle and resources.yaml (spec section 3.3).
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/honeybadger-io/terraform-provider-honeybadger/internal/specgen"
)

func main() {
	spec := flag.String("spec", "", "path to the OpenAPI bundle")
	config := flag.String("config", "", "path to resources.yaml")
	out := flag.String("out", ".", "directory for the generated files")
	api := flag.String("api", "github.com/honeybadger-io/api-go/apiv3", "import path of the client package")
	flag.Parse()
	if *spec == "" || *config == "" {
		fmt.Fprintln(os.Stderr, "usage: specgen -spec bundled.yaml -config resources.yaml [-out dir]")
		os.Exit(2)
	}
	if err := specgen.Generate(*spec, *config, *out, *api); err != nil {
		fmt.Fprintln(os.Stderr, "specgen:", err)
		os.Exit(1)
	}
}

package main

import (
	"context"

	"github.com/redpanda-data/benthos/v4/public/service"

	// Import full suite of FOSS connect plugins
	_ "github.com/redpanda-data/connect/public/bundle/free/v4"
	// Add our custom ghostscript plugin
	_ "github.com/rockwotj/redpanda-connect-ghostscript-plugin"
)

func main() {
	service.RunCLI(context.Background())
}

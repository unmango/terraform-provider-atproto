package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/unmango/terraform-provider-atproto/internal/provider"
)

//go:generate tfplugindocs generate

// version is set by goreleaser at release time via -ldflags.
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.opentofu.org/unmango/atproto",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err)
	}
}

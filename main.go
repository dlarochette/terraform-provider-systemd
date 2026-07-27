package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/dlarochette/terraform-provider-systemd/internal/provider"
)

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "enable debug support for delve")
	flag.Parse()

	opts := providerserver.ServeOpts{
		Address: "registry.terraform.io/dlarochette/systemd",
		Debug:   debug,
	}

	err := providerserver.Serve(context.Background(), provider.New(Version), opts)
	if err != nil {
		log.Fatal(err)
	}
}

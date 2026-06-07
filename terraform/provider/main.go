package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/shiron-dev/melisia/terraform-provider-truenas/internal/provider"
)

func main() {
	var debug bool

	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers")
	flag.Parse()

	opts := providerserver.ServeOpts{
		Address: "registry.terraform.io/shiron-dev/truenas",
		Debug:   debug,
	}

	if err := providerserver.Serve(context.Background(), provider.New("dev"), opts); err != nil {
		log.Fatal(err)
	}
}

package main

import (
	"context"
	"flag"
	"log"

	"github.com/failfailover-cmd/terraform-provider-openprovider/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "set true for delve")
	flag.Parse()

	opts := providerserver.ServeOpts{
		Address: "registry.terraform.io/failfailover-cmd/openprovider",
		Debug:   debug,
	}

	if err := providerserver.Serve(context.Background(), provider.New(version), opts); err != nil {
		log.Fatal(err)
	}
}

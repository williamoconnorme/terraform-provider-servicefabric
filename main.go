package main

import (
	"context"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/williamoconnorme/terraform-provider-servicefabric/internal/provider"
)

// main is the Terraform provider entrypoint.
func main() {
	err := providerserver.Serve(context.Background(), provider.New, providerserver.ServeOpts{
		Address: "registry.terraform.io/williamoconnorme/servicefabric",
	})
	if err != nil {
		log.Fatalf("provider server failed: %v", err)
	}
}

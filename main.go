// Copyright 2024 KubeStellar Authors
// SPDX-License-Identifier: Apache-2.0

// Terraform Provider for KubeStellar
// This provider enables Infrastructure-as-Code control of KubeStellar,
// allowing users to provision clusters, register them with KubeStellar,
// define BindingPolicies, and manage multi-cluster workloads declaratively.

package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/kubestellar/terraform-provider-kubestellar/internal/provider"
)

// Version information - injected at build time
var (
	version = "dev"
	commit  = "none"
)

func main() {
	var debug bool

	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()

	opts := providerserver.ServeOpts{
		Address: "registry.terraform.io/kubestellar/kubestellar",
		Debug:   debug,
	}

	err := providerserver.Serve(context.Background(), provider.New(version), opts)
	if err != nil {
		log.Fatal(err.Error())
	}
}

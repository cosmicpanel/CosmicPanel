package main

import "flag"

// Entrypoint for CosmicPanel daemon. Configures the logger,
// checks any flags that were passed in the boot arguments,
// and checks for a valid license
func main() {
	configPath := *flag.String("config", "config.yml", "Sets the location for the configuration file")
	debug := *flag.Bool("debug", false, "Pass in debug inorder to run CosmicPanel in debug mode")

}

package main

import (
	"flag"

	"github.com/cosmicpanel/CosmicPanel/config"
	"go.uber.org/zap"
)

// Entrypoint for CosmicPanel daemon. Configures the logger,
// checks any flags that were passed in the boot arguments,
// and checks for a valid license
func main() {
	configPath := *flag.String("config", "config.yml", "Sets the location for the configuration file")
	debug := *flag.Bool("debug", false, "Pass in debug inorder to run CosmicPanel in debug mode")
	dnsonly := *flag.Bool("dnsonly", false, "Pass in dnsonly to recieve a dns only license instead of trial license")

	flag.Parse()

	zap.S().Infof("Using configuration file: %s", configPath)

	c, err := config.ReadConfiguration(configPath)
	if err != nil {
		panic(err)
		return
	}

	if debug {
		c.Debug = true
	}

	if err := ConfigureLogging(c.Debug); err != nil {
		panic(err)
	}

	if c.Debug {
		zap.S().Debugw("running in debug mode")
	}

	zap.S().Infof("Checking for CosmicPanel system user...")
	if _, err := c.EnsureUser(); err != nil {
		zap.S().Panicw("Failed to create CosmicPanel system user", zap.Error(err))
	} else {
		zap.S().Infow("Configured system user...")
	}

	// check for valid license
	zap.S().Infof("Checking for vaid license...")
	c.CheckLicense(dnsonly)
	
}

// ConfigureLogging configures the global logger for Zap so that we can call it from any location
// in the code without having to pass around a logger instance
func ConfigureLogging(debug bool) error {
	cfg := zap.NewProductionConfig()
	if debug {
		cfg = zap.NewDevelopmentConfig()
	}

	cfg.Encoding = "console"
	cfg.OutputPaths = []string{
		"stdout",
	}

	logger, err := cfg.Build()
	if err != nil {
		return err
	}

	zap.ReplaceGlobals(logger)

	return nil
}

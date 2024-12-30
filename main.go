package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"sync"

	"github.com/mitchellh/mapstructure"
	"github.com/nornir-automation/gornir/pkg/gornir"
	"github.com/nornir-automation/gornir/pkg/plugins/connection"
	"github.com/nornir-automation/gornir/pkg/plugins/inventory"
	"github.com/nornir-automation/gornir/pkg/plugins/logger"
	"github.com/nornir-automation/gornir/pkg/plugins/output"
	"github.com/nornir-automation/gornir/pkg/plugins/runner"
	"github.com/nornir-automation/gornir/pkg/plugins/task"
	"gopkg.in/yaml.v3"
)

func main() {
	// Define command-line flag for the inventory file
	inventoryFile := flag.String("i", "devices.yaml", "Path to the devices YAML file")
	flag.Parse()

	// Load devices from the YAML file
	inv, err := loadInventory(*inventoryFile)
	if err != nil {
		log.Fatalf("Error loading inventory from file '%s': %v", *inventoryFile, err)
	}

	// Create a Gornir instance
	gn, err := gornir.New(
		&config.Config{},
		inv,
		gornir.WithRunner(goexec.New(&goexec.Options{NumWorkers: runtime.NumCPU()})), // Run tasks concurrently
	)
	if err != nil {
		log.Fatal(err)
	}
	defer gn.Close()

	// Create a wait group to wait for all goroutines to complete
	var wg sync.WaitGroup

	// Execute commands concurrently on each device
	for _, host := range inv.Hosts {
		wg.Add(1)
		go func(h *inventory.Host) {
			defer wg.Done()

			fmt.Printf("--- Executing commands on host: %s (%s) ---\n", h.Hostname, h.Platform)

			// Retrieve commands defined for this host
			cmds, ok := h.Vars["cmds"].([]interface{}) // Commands are loaded as []interface{} from YAML
			if !ok {
				fmt.Printf("No commands defined for host: %s\n", h.Hostname)
				return
			}

			for _, cmdRaw := range cmds {
				cmd, ok := cmdRaw.(string)
				if !ok {
					fmt.Printf("Invalid command format for host %s: %v\n", h.Hostname, cmdRaw)
					continue
				}

				results, err := gn.RunSync(
					context.Background(),
					func(ctx context.Context, host *inventory.Host) (*gornir.JobResult, error) {
						return host.Run(ctx, command.RunCommand(cmd))
					},
					gornir.WithHosts(h.Name), // Target only the current host
				)

				if err != nil {
					fmt.Printf("Error running command '%s' on host '%s': %v\n", cmd, h.Hostname, err)
					continue
				}

				for _, res := range results {
					if res.Error() != nil {
						fmt.Printf("Error executing command '%s' on host '%s': %v\n", cmd, h.Hostname, res.Error())
					} else {
						fmt.Printf("Command: %s\n", cmd)
						fmt.Printf("Output:\n%s\n", res.Result().(*command.CommandResult).Stdout)
						if res.Result().(*command.CommandResult).Stderr != "" {
							fmt.Printf("Stderr:\n%s\n", res.Result().(*command.CommandResult).Stderr)
						}
					}
				}
			}
			fmt.Println()
		}(host)
	}

	// Wait for all commands to finish
	wg.Wait()

	fmt.Println("All commands executed.")
}

// loadInventory loads the inventory from a YAML file.
func loadInventory(filename string) (*inventory.Inventory, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	inv := &inventory.Inventory{
		Hosts: make(map[string]*inventory.Host),
		Groups: make(map[string]*inventory.Group),
	}

	// Define a struct to unmarshal the YAML data
	type yamlInventory struct {
		Hosts map[string]yamlHost `yaml:"hosts"`
		Groups map[string]yamlGroup `yaml:"groups"`
	}

	type yamlHost struct {
		Hostname        string                 `yaml:"hostname"`
		Platform        string                 `yaml:"platform"`
		ConnectionOptions map[string]yamlConnectionOptions `yaml:"connection_options"`
		Cmds            []string               `yaml:"cmds"` // Added cmds field
	}

	type yamlGroup struct {
		Hosts []string `yaml:"hosts"`
	}

	type yamlConnectionOptions map[string]interface{}

	var yInv yamlInventory
	if err := yaml.Unmarshal(data, &yInv); err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML: %w", err)
	}

	// Populate the Gornir inventory from the unmarshaled data
	for name, yHost := range yInv.Hosts {
		host := &inventory.Host{
			Name:            name,
			Hostname:        yHost.Hostname,
			Platform:        yHost.Platform,
			ConnectionOptions: make(map[string]interface{}),
			Vars: map[string]interface{}{
				"cmds": yHost.Cmds, // Store the commands in the Vars field
			},
		}
		for connType, opts := range yHost.ConnectionOptions {
			switch connType {
			case "ssh":
				sshOpts := &ssh.Options{}
				if err := mapstructureDecode(opts, sshOpts); err != nil {
					return nil, fmt.Errorf("failed to decode SSH options for host '%s': %w", name, err)
				}
				host.ConnectionOptions["ssh"] = sshOpts
			case "netconf":
				netconfOpts := &netconf.Options{}
				if err := mapstructureDecode(opts, netconfOpts); err != nil {
					return nil, fmt.Errorf("failed to decode Netconf options for host '%s': %w", name, err)
				}
				host.ConnectionOptions["netconf"] = netconfOpts
			default:
				log.Printf("Warning: Unknown connection type '%s' for host '%s'", connType, name)
			}
		}
		inv.Hosts[name] = host
	}

	for name, yGroup := range yInv.Groups {
		group := &inventory.Group{
			Name: name,
		}
		for _, hostName := range yGroup.Hosts {
			if h, ok := inv.Hosts[hostName]; ok {
				group.Hosts = append(group.Hosts, h)
			} else {
				log.Printf("Warning: Host '%s' not found for group '%s'", hostName, name)
			}
		}
		inv.Groups[name] = group
	}

	// Automatically create the 'all' group if it doesn't exist
	if _, ok := inv.Groups["all"]; !ok {
		allGroup := &inventory.Group{Name: "all", Hosts: []*inventory.Host{}}
		for _, h := range inv.Hosts {
			allGroup.Hosts = append(allGroup.Hosts, h)
		}
		inv.Groups["all"] = allGroup
	}

	return inv, nil
}

// Helper function to decode map[string]interface{} to a struct
func mapstructureDecode(input map[string]interface{}, output interface{}) error {
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName: "yaml",
		Result:  output,
	})
	if err != nil {
		return err
	}
	return decoder.Decode(input)
}

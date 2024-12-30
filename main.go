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
gn, err := gornir.New(&config.Config{}, inv,
	gornir.WithRunner(goexec.New(&goexec.Options{NumWorkers: runtime.NumCPU()})),
)
if err != nil {
	log.Fatal(err)
}
defer gn.Close()

// Execute commands concurrently on each device
var wg sync.WaitGroup
for _, host := range inv.Hosts {
	wg.Add(1)
	go func(h *inventory.Host) {
		defer wg.Done()
		executeCommands(gn, h)
	}(host)
}

// Wait for all commands to finish
wg.Wait()
fmt.Println("All commands executed.")


}

func executeCommands(gn *gornir.Gornir, h *inventory.Host) {
	fmt.Printf("--- Executing commands on host: %s (%s) ---\n", h.Hostname, h.Platform)
	cmds, ok := h.Vars["cmds"].([]interface{})
	if !ok {
		fmt.Printf("No commands defined for host: %s\n", h.Hostname)
		return
	}

for _, cmdRaw := range cmds {
	if cmd, ok := cmdRaw.(string); ok {
		runCommand(gn, h, cmd)
	} else {
		fmt.Printf("Invalid command format for host %s: %v\n", h.Hostname, cmdRaw)
	}
}
fmt.Println()


}

func runCommand(gn *gornir.Gornir, h *inventory.Host, cmd string) {
	results, err := gn.RunSync(context.Background(), 
		func(ctx context.Context, host *inventory.Host) (*gornir.JobResult, error) {
			return host.Run(ctx, command.RunCommand(cmd))
		}, 
		gornir.WithHosts(h.Name), 
	)
	if err != nil {
		fmt.Printf("Error running command '%s' on host '%s': %v\n", cmd, h.Hostname, err)
		return
	}

for _, res := range results {
	if res.Error() != nil {
		fmt.Printf("Error executing command '%s' on host '%s': %v\n", cmd, h.Hostname, res.Error())
	} else {
		printResult(cmd, res)
	}
}


}

func printResult(cmd string, res *gornir.JobResult) {
	cmdResult := res.Result().(*command.CommandResult)
	fmt.Printf("Command: %s\nOutput:\n%s\n", cmd, cmdResult.Stdout)
	if cmdResult.Stderr != "" {
		fmt.Printf("Stderr:\n%s\n", cmdResult.Stderr)
	}
}

func loadInventory(filename string) (*inventory.Inventory, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

var yInv struct {
	Hosts  map[string]yamlHost `yaml:"hosts"`
	Groups map[string]yamlGroup `yaml:"groups"`
}

if err := yaml.Unmarshal(data, &yInv); err != nil {
	return nil, fmt.Errorf("failed to unmarshal YAML: %w", err)
}

inv := &inventory.Inventory{Hosts: map[string]*inventory.Host{}, Groups: map[string]*inventory.Group{}}

// Populate inventory from YAML
for name, yHost := range yInv.Hosts {
	inv.Hosts[name] = &inventory.Host{
		Name:            name,
		Hostname:        yHost.Hostname,
		Platform:        yHost.Platform,
		ConnectionOptions: parseConnectionOptions(yHost.ConnectionOptions),
		Vars: map[string]interface{}{"cmds": yHost.Cmds},
	}
}

for name, yGroup := range yInv.Groups {
	inv.Groups[name] = &inventory.Group{Name: name}
	for _, hostName := range yGroup.Hosts {
		if h, ok := inv.Hosts[hostName]; ok {
			inv.Groups[name].Hosts = append(inv.Groups[name].Hosts, h)
		}
	}
}

createAllGroup(inv)
return inv, nil


}

func parseConnectionOptions(options map[string]yamlConnectionOptions) map[string]interface{} {
	connOptions := make(map[string]interface{})
	for connType, opts := range options {
		if connType == "ssh" {
			sshOpts := &ssh.Options{}
			if err := mapstructureDecode(opts, sshOpts); err == nil {
				connOptions["ssh"] = sshOpts
			}
		} else if connType == "netconf" {
			netconfOpts := &netconf.Options{}
			if err := mapstructureDecode(opts, netconfOpts); err == nil {
				connOptions["netconf"] = netconfOpts
			}
		}
		log.Printf("Warning: Unknown connection type '%s' for host '%s'", connType, opts)
	}
	return connOptions
}

func createAllGroup(inv *inventory.Inventory) {
	if _, ok := inv.Groups["all"]; !ok {
		allGroup := &inventory.Group{Name: "all"}
		for _, h := range inv.Hosts {
			allGroup.Hosts = append(allGroup.Hosts, h)
		}
		inv.Groups["all"] = allGroup
	}
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

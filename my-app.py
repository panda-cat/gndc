from nornir import InitNornir
from nornir_netmiko import netmiko_send_multiline
import os

def execute_commands(task):
    device_output = ""
    for command in task.host["commands"]:
        result = task.run(task=netmiko_send_multiline, command_string=command)
        device_output += f"Command: {command}\n"
        device_output += result.result
        device_output += "\n" + "-"*40 + "\n"
    
    # Save the output to a separate file for each device
    with open(f"{task.host.hostname}.txt", "w") as file:
        file.write(f"Device: {task.host.name}\n")
        file.write(device_output)

def main():
    # Initialize Nornir
    nr = InitNornir(config_file="config.yaml")
    
    # Execute commands on each device
    nr.run(task=execute_commands)

if __name__ == "__main__":
    main()

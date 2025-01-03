from nornir import InitNornir
from nornir_utils.plugins.functions import print_result
from nornir_netmiko import netmiko_send_command
from nornir_scrapli.tasks import send_command
from datetime import datetime
import os


nr = InitNornir(
    runner={
        "plugin": "threaded",
        "options": {
            "num_workers": 100,
        },
    },
    inventory={
        "plugin": "SimpleInventory",
        "options": {
            "host_file": "inventory/hosts.yaml",
            "group_file": "inventory/groups.yaml"
        },
    },
)


# 创建以当前日期命名的文件夹
today = datetime.now().strftime("%Y-%m-%d")
if not os.path.exists(today):
    os.makedirs(today)

# 初始化 Nornir
nr = InitNornir(config_file="config.yaml")

def execute_commands(task):
    # 获取设备的自定义命令，如果未定义则使用默认命令
    custom_commands = task.host.get("commands")
    # 根据不同的平台选择不同的连接插件和命令
    platform = task.host.platform
    if custom_commands:
        commands_to_run = custom_commands
    else:
      commands_to_run = {
          "huawei": [
              "display version",
              "display interface brief",
              "display current-configuration",
          ],
          "h3c": [
              "display version",
              "display interface brief",
              "display current-configuration",
          ],
          "cisco": [
              "show version",
              "show ip interface brief",
              "show running-config",
          ],
          "cisco_asa": [
              "show version",
              "show interface ip brief",
              "show running-config",
          ],
          "paloalto": [
              "show system info",
              "show interface all",
              "show config running",
          ],
          "fortinet": [
              "get system status",
              "show full-configuration",
              "get system interface",
          ],
          "f5": [
              "show sys version",
              "show running-config",
              "show sys interface",
          ],
          "ruckus_icx": [
              "show version",
              "show interface brief",
              "show running-config",
          ],
      }.get(platform, [])

    if platform in ["huawei", "h3c", "cisco", "cisco_asa", "ruckus_icx"]:
        # 使用 Netmiko
        for command in commands_to_run:
            result = task.run(task=netmiko_send_command, command_string=command)
            save_output(task, command, result)
    elif platform in ["paloalto", "fortinet", "f5"]:
        # 使用 Scrapli
        for command in commands_to_run:
            result = task.run(task=send_command, command=command)
            save_output(task, command, result)
    else:
        print(f"不支持的设备类型: {platform}")

def save_output(task, command, result):
    # 将命令和输出写入文件
    filename = f"{today}/{task.host}.txt"
    with open(filename, "a") as f:
        f.write(f"--- {command} ---\n")
        if isinstance(result.result, str):
          f.write(result.result + "\n\n")
        else:
          f.write(str(result.result) + "\n\n")

# 执行任务
result = nr.run(task=execute_commands)

# 打印结果
print_result(result)

print(f"命令执行结果已保存至 '{today}' 文件夹")

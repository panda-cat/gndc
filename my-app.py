import argparse
import os
import datetime
from nornir import InitNornir
from nornir.core.task import Task, Result
from nornir_netmiko import netmiko_send_command, netmiko_send_config
from nornir_scrapli import send_command
from nornir.core.exceptions import ConnectionException
from scrapli.driver import register_driver
import yaml

# 注册社区驱动
register_driver("huawei_vrp")
register_driver("h3c_vrp")

def get_commands(task: Task, commands_data):
    """根据设备类型获取要执行的命令列表"""
    commands = []
    for group in task.host.groups:
        if group in commands_data:
            commands.extend(commands_data[group])
    return commands

def connect_and_authenticate(task: Task) -> Result:
    """连接设备并处理认证"""
    # Netmiko 连接参数
    netmiko_extras = {}
    if task.host.get("enable_password"):
        netmiko_extras["secret"] = task.host["enable_password"]
    if task.host.get("enable_username"):
        netmiko_extras["global_delay_factor"] = 2  # 可选：增加延迟
        netmiko_extras["username"] = task.host["enable_username"]

    # Scrapli 连接参数 (如果需要)
    scrapli_extras = {}

    try:
        # 根据平台选择合适的连接方式
        if "netmiko" in task.host.platform:
            task.host.open(
                hostname=task.host.hostname,
                username=task.host.username,
                password=task.host.password,
                platform=task.host.platform,
                extras=netmiko_extras,
                default_to_enable_mode=False
            )
            if "ruckus_icx" == task.host.platform:
                # Ruckus ICX 特殊处理：进入 enable 模式
                if task.host.get("enable_username"):
                    task.run(task=netmiko_send_config, config_commands=[
                             "enable", task.host["enable_username"], task.host["enable_password"]])
                else:
                    task.run(task=netmiko_send_config, config_commands=["enable", task.host["enable_password"]])

        elif "telnet" in task.host.platform:
            # 对于telnet，特殊处理平台名称
            platform = task.host.platform.replace("_telnet", "")
            task.host.platform = platform
            task.host.open(
                hostname=task.host.hostname,
                username=task.host.username,
                password=task.host.password,
                platform=task.host.platform,
                extras=netmiko_extras,
                default_to_enable_mode=False
            )
            # Cisco 特殊处理：进入 enable 模式
            task.run(task=netmiko_send_config, config_commands=["enable", task.host["enable_password"]])
        else:  # 尝试使用 Scrapli
            task.host.open(
                hostname=task.host.hostname,
                username=task.host.username,
                password=task.host.password,
                platform=task.host.platform,
                extras=scrapli_extras,
                default_to_enable_mode=False # Scrapli 不支持该参数，需要根据设备类型处理
            )

        return Result(host=task.host, result="Successfully connected and authenticated")

    except ConnectionException as e:
        return Result(host=task.host, result=f"Connection failed: {e}", failed=True)
    except Exception as e:
        return Result(host=task.host, result=f"An error occurred: {e}", failed=True)

def execute_commands(task: Task, commands_data: dict) -> Result:
    """连接设备并执行命令"""
    # 首先执行连接和认证任务
    connect_result = task.run(task=connect_and_authenticate)
    if connect_result.failed:
        return Result(
            host=task.host,
            result="Failed to connect and authenticate, skipping commands.",
            failed=True,
        )

    commands = get_commands(task, commands_data)

    results = []
    for command in commands:
        try:
            # 根据平台选择合适的插件
            if "netmiko" in task.host.platform or "telnet" in task.host.platform:
                result = task.run(task=netmiko_send_command, command_string=command)
            else:  # 尝试使用 Scrapli
                result = task.run(task=send_command, command=command)

            results.append(result[0].result)

        except Exception as e:
            results.append(f"Error executing command '{command}': {e}")
            task.results.failed = True

    return Result(host=task.host, result="\n".join(results))

def save_results(task: Task, output_dir: str) -> Result:
    """将命令执行结果保存到文件"""
    filepath = os.path.join(output_dir, f"{task.host.hostname}.txt")
    with open(filepath, "w") as f:
        f.write(task.results[0].result)

    return Result(host=task.host, result=f"Results saved to {filepath}")

def main():
    # 解析命令行参数
    parser = argparse.ArgumentParser(description="Connect to devices and execute commands.")
    parser.add_argument("-i", "--inventory", required=True, help="Path to the inventory file (deves.yaml)")
    args = parser.parse_args()

    # 创建以当前日期命名的文件夹
    current_date = datetime.datetime.now().strftime("%Y-%m-%d")
    output_dir = os.path.join(os.getcwd(), current_date)
    os.makedirs(output_dir, exist_ok=True)

    # 初始化 Nornir
    nr = InitNornir(
        config_file="config.yaml",  # 可以选择使用配置文件，也可以不使用
        inventory={
            "plugin": "nornir.core.plugins.inventory.simple.SimpleInventory",
            "options": {
                "host_file": args.inventory,
            },
        },
    )

    # 加载命令清单文件
    with open("tasks/commands.yaml", "r") as f:
        commands_data = yaml.safe_load(f)

    # 执行任务
    nr.run(task=execute_commands, commands_data=commands_data)
    nr.run(task=save_results, output_dir=output_dir)

if __name__ == "__main__":
    main()

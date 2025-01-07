import sys
import logging
from nornir import InitNornir
from nornir.core.task import Result
from nornir_netmiko import netmiko_send_command
from nornir_utils.plugins.tasks.files import write_file
from nornir.plugins.inventory.simple import SimpleInventory


# 初始化Nornir
nr = InitNornir(config_file="config.yaml")

# 设置日志
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger("nornir")

def execute_commands(task):
    # 从hosts文件中读取每台设备的命令列表
    commands = task.host["commands"]
    results = []

    # 逐条执行命令并保存结果
    for command in commands:
        result = task.run(netmiko_send_command, command_string=command)
        results.append(result.result)
    
    # 将结果写入文件，以IP地址命名
    filename = f"{task.host.hostname}.txt"
    task.run(write_file, content="\n".join(results), filename=filename)

    return Result(host=task.host, result="Commands executed and saved")

# 运行任务
try:
    nr.run(task=execute_commands)
except Exception as e:
    logger.error(f"An error occurred: {str(e)}")
    sys.exit(1)


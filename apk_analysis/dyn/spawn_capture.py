#!/usr/bin/env python3
"""spawn 冷启 com.phoenix.read 并挂着 playback hook，把命中行写到日志。

用法: python3 capture_driver.py <script.js> <log> <seconds>
"""
import sys
import time

import frida

DEV = "127.0.0.1:18888"
PKG = "com.phoenix.read"


def main():
    script_path, log_path = sys.argv[1], sys.argv[2]
    seconds = int(sys.argv[3]) if len(sys.argv) > 3 else 180
    with open(script_path, encoding="utf-8") as f:
        source = f.read()
    log = open(log_path, "w", buffering=1, encoding="utf-8")

    def write(line):
        log.write(line + "\n")

    device = frida.get_device_manager().add_remote_device(DEV)
    try:
        device.kill(PKG)
    except Exception:  # 进程没在跑就不用 kill
        pass
    time.sleep(1)
    pid = device.spawn([PKG])
    session = device.attach(pid)
    script = session.create_script(source)
    script.on("message", lambda message, data: write("MSG " + str(message)))
    script.load()
    device.resume(pid)
    write("SPAWNED pid=%d" % pid)
    deadline = time.time() + seconds
    while time.time() < deadline:
        time.sleep(1)
    write("DONE")
    log.close()


if __name__ == "__main__":
    main()

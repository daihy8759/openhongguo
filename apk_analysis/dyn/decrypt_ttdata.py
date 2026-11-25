#!/usr/bin/env python3
"""解开注册请求的 tt-data 信封（magic 74 63 05 10 00 00 + 32B 前缀 + AES-CBC）。

用法:
  adb ... > /dev/null; 先用 spawn_capture.py + hook_register_playback.js 抓 REQBODY，
  再把 base64 喂进来:
      python3 decrypt_ttdata.py '<base64>'
  或从抓包日志里自动挑第一条 device_register 的 body:
      python3 decrypt_ttdata.py --log net.out

信封结构（与本仓库 internal/protocol/tt_encrypt.go 同一套常量）：
  prefix = body[6:38]                 # 官方客户端每次随机，我们固定
  key/iv = sha512(sha512(prefix) || CONST)  前 16 / 16..32 字节
  明文   = AES-128-CBC-decrypt(body[38:]) → 去 PKCS#7 → 去掉前 64 字节 SHA-512 → gzip → JSON
"""
import base64
import gzip
import hashlib
import json
import sys

from cryptography.hazmat.primitives.ciphers import Cipher, algorithms, modes

CONST = bytes.fromhex(
    "4dd4c2e6b83162090e52b3c7a6733ba41cb2462b829ab58a196b39db57177524f4"
    "9baf7f08e8d68d26a72e37c1a95a2f1f05a51892aef2949732b62a38aadd58"
)
MAGIC = bytes([0x74, 0x63, 0x05, 0x10, 0x00, 0x00])


def decrypt(body: bytes) -> bytes:
    if body[:6] != MAGIC:
        raise ValueError("not a tt-data envelope: %s" % body[:6].hex())
    prefix, ct = body[6:38], body[38:]
    kh = hashlib.sha512(prefix).digest()
    ki = hashlib.sha512(kh + CONST).digest()
    dec = Cipher(algorithms.AES(ki[:16]), modes.CBC(ki[16:32])).decryptor()
    plain = dec.update(ct) + dec.finalize()
    plain = plain[: -plain[-1]]
    plain = plain[64:]
    if plain[:2] == b"\x1f\x8b":
        plain = gzip.decompress(plain)
    return plain


def from_log(path: str) -> bytes:
    pending = False
    for line in open(path, encoding="utf-8", errors="replace"):
        if line.startswith("REQ POST") and "device_register" in line:
            pending = True
        elif line.startswith("REQBODY ") and pending:
            return base64.b64decode(line.split()[1])
    raise SystemExit("no device_register body in %s" % path)


def main() -> int:
    if len(sys.argv) < 2:
        print(__doc__)
        return 1
    raw = from_log(sys.argv[2]) if sys.argv[1] == "--log" else base64.b64decode(sys.argv[1])
    print(json.dumps(json.loads(decrypt(raw)), ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

# 官方 SDK 编码对照数据

配置与测试向量来自用户提供的《移动云盘领取奖励和真实抢兑.har》entries[1244] 中的 `fp.min.js`，SDK 3.0.0。HAR SHA256：`583de610e5f89a9569fef6485ca68002cc374c9ecb7a17a473c0e0aab275eec4`。

仅在隔离 JavaScript 环境中加载原 SDK 的配置、DES、AES、gzip、字段映射和摘要函数。跳过 SDK 启动，不运行浏览器指纹采集，不联网；输入均为虚构文本和固定测试 UID，没有提取 HAR 的账号、Cookie、设备 ID 或会话密钥。

`official_codec_vectors.json` 包括：

- DES-ECB 零填充的逐字节输出（包含 UTF-16 中文和代理对）。
- AES-CBC、ASCII IV、零填充的逐字节输出，包含整块长度与跨块长度。
- 官方摘要、字段映射、完整 AES(gzip-base64(JSON)) 输出。

Go 与 pako 的合法 DEFLATE 流、JSON 键顺序可能不同，完整链路比较解密解压后的对象；DES、AES 和摘要直接比对原 SDK 输出。`../sdk_config.json` 中的字段密钥是分发给浏览器的公开协议常量，不是账号秘密。

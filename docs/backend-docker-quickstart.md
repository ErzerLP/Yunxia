# Yunxia 后端 Docker 启动说明

## 1. 适用范围

本说明只覆盖后端容器化启动：

- `backend`
- `aria2`

对应编排文件：

- `docker-compose.backend.yml`

## 2. 启动前准备

在项目根目录执行：

```powershell
Copy-Item backend/.env.example backend/.env
```

如需调整宿主机端口或 JWT 密钥，编辑：

- `backend/.env`

## 3. 启动命令

```powershell
docker compose --env-file backend/.env -f docker-compose.backend.yml up -d --build
```

## 4. 验证命令

查看容器状态：

```powershell
docker compose -f docker-compose.backend.yml ps
```

验证后端健康检查：

```powershell
Invoke-WebRequest -UseBasicParsing http://127.0.0.1:8080/api/v1/health
```

验证 Aria2 RPC：

```powershell
$body = '{"jsonrpc":"2.0","id":"check","method":"aria2.getVersion","params":[]}'
Invoke-RestMethod -Method Post -Uri http://127.0.0.1:6800/jsonrpc -ContentType 'application/json' -Body $body
```

## 5. 停止命令

```powershell
docker compose -f docker-compose.backend.yml down
```

如需同时删除命名卷：

```powershell
docker compose -f docker-compose.backend.yml down -v
```

## 6. 当前已知说明

- backend 数据卷挂载到 `/app/data`
- Aria2 配置目录为 `/config`
- Aria2 下载目录为 `/downloads`
- backend 与 aria2 共享 `/downloads`
- 如果希望 Yunxia 直接浏览 Aria2 下载结果，需要在系统里手动创建一个 `base_path=/downloads` 的 local source

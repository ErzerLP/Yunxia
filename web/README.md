# Yunxia Web

这是 Yunxia 的前端应用，基于 React、TypeScript、Vite、Tailwind CSS、TanStack Query、Zustand 与 Axios 构建。

完整项目介绍、后端启动与部署说明请优先阅读根目录 [`../README.md`](../README.md)。前后端协作时可参考 [`../backend/API_CONTRACT.md`](../backend/API_CONTRACT.md)、[`../backend/FRONTEND_HANDOFF.md`](../backend/FRONTEND_HANDOFF.md) 与 [`FRONTEND_TEST_HANDOFF.md`](FRONTEND_TEST_HANDOFF.md)。

## 快速启动

先在仓库根目录启动后端：

```powershell
docker compose --env-file backend/.env -f docker-compose.backend.yml up -d --build
```

再启动前端：

```powershell
cd web
npm install
npm run dev
```

默认访问：

```text
http://127.0.0.1:5173
```

开发期代理配置位于 `vite.config.ts`：

- `/api/*` → `http://localhost:8080`
- `/dav/*` → `http://localhost:8080`
- `/__public_share/*` → 后端 `/s/*`

## 常用命令

```powershell
npm run dev      # Vite dev server
npm run lint     # ESLint
npm run build    # TypeScript build + Vite build
npm run preview  # 静态产物预览
```

注意：`npm run preview` 不会自动代理 `/api` 到后端；对外测试建议使用 Vite dev server 或额外配置同源反向代理。

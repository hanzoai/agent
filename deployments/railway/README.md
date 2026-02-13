# Hanzo Agents Railway Deployment

Deploy Hanzo Agents control plane with PostgreSQL and agent nodes on Railway using Docker images.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Railway Project                          │
│                                                              │
│  ┌──────────────────┐    ┌──────────────────┐               │
│  │  Control Plane   │    │    PostgreSQL    │               │
│  │  (Docker Image)  │───▶│   (with pgvector)│               │
│  │                  │    │                  │               │
│  │  - Web UI        │    └──────────────────┘               │
│  │  - REST API      │                                       │
│  │  - Agent Registry│                                       │
│  └────────┬─────────┘                                       │
│           │                                                  │
│           ▼                                                  │
│  ┌──────────────────┐                                       │
│  │   Agent Node     │                                       │
│  │  (Docker Image)  │                                       │
│  │                  │                                       │
│  │  - Reasoners     │                                       │
│  │  - Skills        │                                       │
│  └──────────────────┘                                       │
└─────────────────────────────────────────────────────────────┘
```

## Quick Setup

### 1. Create a New Railway Project

Go to [railway.app](https://railway.app) and create a new empty project.

### 2. Add PostgreSQL

1. Click **New** → **Database** → **Add PostgreSQL**
2. Railway will provision a PostgreSQL instance automatically

### 3. Deploy Control Plane

1. Click **New** → **Docker Image**
2. Enter: `ghcr.io/hanzoai/agents:latest`
3. Add these environment variables:

| Variable | Value | Description |
|----------|-------|-------------|
| `HANZO_AGENTS_STORAGE_MODE` | `postgres` | Use PostgreSQL backend |
| `HANZO_AGENTS_STORAGE_POSTGRES_URL` | `${{Postgres.DATABASE_URL}}` | Auto-wired from Railway |
| `HANZO_AGENTS_API_KEY` | (generate a secure key) | API key for authentication |

4. In **Settings** → **Networking**, click **Generate Domain** to get a public URL
5. Deploy - the control plane will auto-migrate the database on startup

### 4. Deploy an Agent Node (Optional)

1. Click **New** → **Docker Image**
2. Enter: `ghcr.io/hanzoai/agents-init-example:latest`
3. Add these environment variables:

| Variable | Value | Description |
|----------|-------|-------------|
| `HANZO_AGENTS_URL` | `http://${{control-plane.RAILWAY_PRIVATE_DOMAIN}}:8080` | Internal URL to control plane |
| `HANZO_AGENTS_API_KEY` | (same as control plane) | Must match control plane key |
| `AGENT_CALLBACK_URL` | `http://${{RAILWAY_PRIVATE_DOMAIN}}:8005` | URL for control plane to reach this agent |
| `PORT` | `8005` | Agent server port |
| `OPENAI_API_KEY` | (your key) | Optional - for AI reasoners |

> **Note:** Replace `control-plane` with your control plane service name if different. The `AGENT_CALLBACK_URL` is critical - without it, the agent will show as "offline" in the UI because the control plane can't reach it for health checks.

## Environment Variables Reference

### Control Plane

| Variable | Required | Description |
|----------|----------|-------------|
| `HANZO_AGENTS_STORAGE_MODE` | Yes | Set to `postgres` for PostgreSQL |
| `HANZO_AGENTS_STORAGE_POSTGRES_URL` | Yes | PostgreSQL connection string |
| `HANZO_AGENTS_API_KEY` | Recommended | API key for authentication |
| `HANZO_AGENTS_UI_ENABLED` | No | Enable web UI (default: true) |

### Agent Node

| Variable | Required | Description |
|----------|----------|-------------|
| `HANZO_AGENTS_URL` | Yes | URL to control plane |
| `HANZO_AGENTS_API_KEY` | Yes* | Must match control plane key |
| `AGENT_CALLBACK_URL` | Yes | URL for control plane to reach this agent for health checks |
| `PORT` | No | Agent HTTP port (default: 8005) |
| `AGENT_ID` | No | Custom agent ID |

*Required if control plane has `HANZO_AGENTS_API_KEY` set.

## Testing Your Deployment

Once deployed, test the agent via the control plane:

```bash
# Set your control plane URL
export CP_URL=https://your-control-plane.up.railway.app

# Echo reasoner (no AI needed)
curl -X POST $CP_URL/api/v1/execute/init-example.demo_echo \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-api-key" \
  -d '{"input": {"message": "Hello Railway!"}}'

# Sentiment analysis (requires OPENAI_API_KEY on agent)
curl -X POST $CP_URL/api/v1/execute/init-example.demo_analyzeSentiment \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-api-key" \
  -d '{"input": {"text": "I love this deployment!"}}'
```

## Run Agent Locally

Connect a local agent to your Railway control plane:

```bash
# Using the CLI
curl -sSf https://hanzo-agents.ai/get | sh
af init my-agent
cd my-agent

export HANZO_AGENTS_SERVER=https://your-control-plane.up.railway.app
export HANZO_AGENTS_API_KEY=your-api-key
af run

# Or run an example directly
git clone https://github.com/hanzoai/agents.git
cd hanzo-agents/examples/ts-node-examples/init-example
npm install
HANZO_AGENTS_URL=https://your-control-plane.up.railway.app \
HANZO_AGENTS_API_KEY=your-api-key \
npm start
```

## Local Development

For local development with Docker Compose:

```bash
git clone https://github.com/hanzoai/agents.git
cd hanzo-agents/deployments/docker
docker compose up
```

## Resources

- [Documentation](https://github.com/hanzoai/agents)
- [Examples](https://github.com/hanzoai/agents/tree/main/examples)
- [Python SDK](https://pypi.org/project/hanzo-agents/)
- [TypeScript SDK](https://www.npmjs.com/package/@hanzo-agents/sdk)

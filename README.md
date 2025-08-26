# FlowGuard

A rate limiting and API gateway service built in Go. Acts as a reverse proxy that enforces rate limits, validates API keys, and forwards requests to upstream services with comprehensive monitoring and observability.

---

## Architecture

```mermaid
graph TD
    %% Client Layer
    subgraph "Client Layer"
        A1["API Client"] -->|"HTTP/HTTPS"| A2["Load Balancer<br>(Optional)"]
        A2 -->|"Requests"| A3["Rate Limiting Gateway<br>(Port 8080)"]
    end

    %% Gateway Layer
    subgraph "Gateway Layer (Go)"
        B1["HTTP Server<br>(Gin Framework)"] -->|"Request Processing"| B2["Middleware Chain"]
        B2 -->|"Authentication"| B3["Auth Middleware<br>(API Key Validation)"]
        B2 -->|"Rate Limiting"| B4["Rate Limit Middleware<br>(Sliding Window)"]
        B2 -->|"Logging"| B5["Logging Middleware<br>(Structured JSON)"]
        B2 -->|"Metrics"| B6["Metrics Middleware<br>(Prometheus)"]
        B2 -->|"Proxy"| B7["Gateway Handler<br>(Request Forwarding)"]
    end

    %% Storage Layer
    subgraph "Storage Layer"
        C1["Redis<br>(Rate Limit Counters)"] -->|"Atomic Operations"| C2["Rate Limit Storage<br>(Per-minute, Hour, Day)"]
        C1 -->|"Quota Tracking"| C3["Monthly Quota<br>(Per API Key)"]
        C1 -->|"Key Validation"| C4["Valid API Keys<br>(Optional Set)"]
    end

    %% Upstream Layer
    subgraph "Upstream Services"
        D1["Target Service<br>(Configurable URL)"] -->|"Proxied Requests"| D2["Response Processing<br>(Headers & Body)"]
        D3["Health Check<br>(/health)"] -->|"Service Status"| D4["Monitoring<br>(Prometheus)"]
    end

    %% Observability Layer
    subgraph "Observability Layer"
        E1["Prometheus Metrics<br>(/metrics)"] -->|"Time Series Data"| E2["Monitoring Dashboard<br>(Grafana)"]
        E3["Structured Logs<br>(JSON Format)"] -->|"Log Aggregation"| E4["Log Analysis<br>(ELK Stack)"]
        E5["Rate Limit Headers<br>(X-RateLimit-*)"] -->|"Client Feedback"| E6["Client Monitoring<br>(Usage Tracking)"]
    end

    %% Cross-layer connections
    A3 -->|"Authenticated Requests"| B1
    B3 -->|"Validate Keys"| C4
    B4 -->|"Check Limits"| C2
    B4 -->|"Check Quota"| C3
    B7 -->|"Forward Requests"| D1
    B6 -->|"Record Metrics"| E1
    B5 -->|"Log Events"| E3
    B4 -->|"Set Headers"| E5
```

---

## System Overview

The Rate Limiting API Gateway implements a high-performance, distributed architecture with clear separation of concerns across five main layers:

### **Client Layer**

- **API Clients**: External applications making requests to protected services
- **Load Balancer**: Optional reverse proxy for high availability and SSL termination
- **Request Routing**: Intelligent routing based on API keys and rate limits

### **Gateway Layer**

- **HTTP Server**: Gin-based web server with graceful shutdown and health checks
- **Middleware Chain**: Modular middleware for authentication, rate limiting, logging, and metrics
- **Authentication**: API key validation with Redis-based key storage
- **Rate Limiting**: Multi-level rate limiting (minute, hour, day, monthly quota)
- **Request Proxy**: Intelligent forwarding with header preservation and error handling

### **Storage Layer**

- **Redis**: High-performance in-memory storage for rate limit counters and quotas
- **Atomic Operations**: Redis pipelines for consistent rate limit increments
- **Distributed Storage**: Shared state across multiple gateway instances
- **Key Management**: Optional Redis set for valid API key validation

### **Upstream Services**

- **Target Services**: Configurable upstream services (APIs, microservices, etc.)
- **Response Processing**: Header preservation and gateway-specific header injection
- **Error Handling**: Comprehensive error handling with appropriate HTTP status codes
- **Timeout Management**: Configurable timeouts for upstream service calls

### **Observability Layer**

- **Prometheus Metrics**: Comprehensive metrics for monitoring and alerting
- **Structured Logging**: JSON-formatted logs with request/response details
- **Rate Limit Headers**: Client-facing headers for rate limit status
- **Health Checks**: Built-in health check endpoints for load balancers

The system supports multiple deployment patterns:

1. **Single Instance**: Direct deployment for development and testing
2. **Load Balanced**: Multiple instances behind a load balancer
3. **Kubernetes**: Containerized deployment with horizontal scaling
4. **Docker Compose**: Local development with Redis and optional upstream services

---

## Data Flow

```mermaid
sequenceDiagram
    participant C as Client
    participant G as Gateway
    participant R as Redis
    participant U as Upstream
    participant M as Metrics

    %% Request Processing
    C->>G: HTTP Request with X-API-Key
    G->>G: Log Request Start
    G->>G: Validate API Key

    alt Invalid API Key
        G-->>C: 401 Unauthorized
    else Valid API Key
        G->>R: Check Rate Limits
        R-->>G: Current Usage

        alt Rate Limit Exceeded
            G-->>C: 429 Too Many Requests
            G->>M: Record Rate Limit Violation
        else Within Limits
            G->>R: Increment Counters
            G->>U: Forward Request
            U-->>G: Response
            G->>G: Add Gateway Headers
            G->>M: Record Success Metrics
            G-->>C: Proxied Response
        end
    end

    G->>G: Log Request End
```

---

## Rate Limiting Strategy

```mermaid
graph LR
    subgraph "Rate Limit Windows"
        A1["Per-Minute<br>(60 requests)"] -->|"Sliding Window"| A2["Redis Counter<br>(rate:key:window)"]
        A3["Per-Hour<br>(1000 requests)"] -->|"Sliding Window"| A4["Redis Counter<br>(rate:key:window)"]
        A5["Per-Day<br>(10000 requests)"] -->|"Sliding Window"| A6["Redis Counter<br>(rate:key:window)"]
        A7["Monthly Quota<br>(100000 requests)"] -->|"Monthly Reset"| A8["Redis Counter<br>(quota:key:month)"]
    end

    subgraph "Rate Limit Headers"
        B1["X-RateLimit-Limit"] -->|"Current Limit"| B2["Client Feedback"]
        B3["X-RateLimit-Remaining"] -->|"Remaining Requests"| B2
        B4["X-RateLimit-Reset"] -->|"Reset Time"| B2
        B5["X-RateLimit-Window"] -->|"Current Window"| B2
    end

    A2 -->|"Check & Increment"| B1
    A4 -->|"Check & Increment"| B1
    A6 -->|"Check & Increment"| B1
    A8 -->|"Check & Increment"| B1
```

---

## Features

- **API Key Authentication**: Secure API key validation with Redis-based storage
- **Multi-Level Rate Limiting**: Per-minute, per-hour, per-day, and monthly quota enforcement
- **Comprehensive Monitoring**: Prometheus metrics for requests, rate limits, and upstream errors
- **Structured Logging**: JSON-formatted logs with request details and API key masking
- **Health Checks**: Built-in health check endpoints for load balancer integration
- **Request Proxy**: Intelligent request forwarding with header preservation
- **Rate Limit Headers**: Client-facing headers for rate limit status and reset times
- **High Performance**: Redis-based distributed rate limiting with atomic operations
- **Docker Support**: Complete containerization with multi-stage builds
- **Kubernetes Ready**: Production-ready deployment with health checks and graceful shutdown
- **Configurable**: Environment-based configuration for all settings
- **Comprehensive Testing**: Unit tests with mocking and integration tests
- **Observability**: Metrics, logs, and rate limit information endpoints

# Minimalist Immutable Ledger Service (MILS)

This project is a lightweight, single-purpose microservice written in Go that provides an immutable, tamper-proof log of transactions. It's designed to be a reliable backend component for auditing, critical event tracking, or any system that requires a high degree of data integrity.

## What Has Been Done

The current implementation includes the following features:

*   **Core Blockchain Logic**: A fully functional blockchain implementation with blocks, transactions, and a Proof-of-Work (PoW) consensus mechanism.
*   **Dynamic Difficulty Adjustment**: The PoW difficulty automatically adjusts every 10 blocks to maintain a consistent block generation time.
*   **Merkle Tree**: Each block includes a Merkle Tree root to ensure the integrity of its transactions and allow for efficient verification.
*   **Externalized Configuration**: Key parameters (difficulty, block time, etc.) are managed via environment variables and a `.env` file for easy configuration across different environments.
*   **Persistence with an Embedded Database**: The blockchain state is persisted using **BoltDB**, an embedded key-value store. For containerized deployments, the database file is stored in a named Docker volume.
*   **Observability**: The service is instrumented for production-level monitoring:
    *   **Structured Logging**: Uses `logrus` for structured, JSON-formatted logging.
    *   **Prometheus Metrics**: Exposes key metrics on a `/metrics` endpoint, including `block_height`, `mempool_size`, and `mining_time_seconds`.
*   **Containerization**: The entire application stack can be run using Docker and Docker Compose, including the ledger service, Prometheus, and a pre-configured Grafana dashboard.
*   **API Endpoints**: A set of RESTful API endpoints to interact with the ledger.

## Future Improvements

This project provides a solid foundation, but there are several areas where it could be extended:

*   **Migrate to a Client-Server Database**: To better align with a microservices architecture, the embedded BoltDB could be replaced with a client-server database like **Redis**. This would allow the database to run in a separate container, fully independent of the application container.
*   **Peer-to-Peer Networking**: To create a truly decentralized ledger, a peer-to-peer networking layer could be added to allow multiple nodes to communicate and stay in sync.
*   **WebSockets**: For real-time updates, a WebSocket API could be added to notify clients when new blocks are mined or new transactions are added to the mempool.

## Start-Up Guide

### Using Docker (Recommended)

The easiest way to run the entire stack is with Docker Compose.

1.  **Clone the repository**:

    ```bash
    git clone https://github.com/joseferreira/Immutable-Ledger-Service.git
    cd Immutable-Ledger-Service
    ```

2.  **Configure Environment**: Copy the example environment file. You can customize the values in `.env` if you wish.

    ```bash
    cp .env.example .env
    ```

3.  **Run the services**:

    ```bash
    docker-compose up --build
    ```

    This will build the ledger service and start the `ledger`, `prometheus`, and `grafana` containers. The configuration from your `.env` file will be automatically loaded.

4.  **Access the services**:

    *   **Ledger API**: `http://localhost:8080`
    *   **Prometheus**: `http://localhost:9090`
    *   **Grafana**: `http://localhost:3000` (a pre-configured dashboard will be available)

### Running Locally

To run the service locally without Docker, you'll need to have Go installed (version 1.22 or later).

1.  **Clone the repository**:

    ```bash
    git clone https://github.com/joseferreira/Immutable-Ledger-Service.git
    cd Immutable-Ledger-Service
    ```

2.  **Configure Environment**: Copy the example environment file. The `godotenv` library will automatically load this file when you run the application.

    ```bash
    cp .env.example .env
    ```

3.  **Install dependencies**:

    ```bash
    go mod tidy
    ```

4.  **Run the service**:

    ```bash
    go run cmd/ledger/main.go
    ```

    The service will start and listen on port `8080`.

### Interacting with the API

You can use a tool like `curl` to interact with the API.

*   **Add a transaction**:

    ```bash
    curl -X POST -d '{"data": "your transaction data"}' http://localhost:8080/transaction
    ```

*   **Mine a new block**:

    ```bash
    curl -X POST http://localhost:8080/mine
    ```

*   **Verify the chain**:

    ```bash
    curl http://localhost:8080/chain/verify
    ```

*   **Get a block by index**:

    ```bash
    curl http://localhost:8080/block/1
    ```

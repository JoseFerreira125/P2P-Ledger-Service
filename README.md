# Minimalist Immutable Ledger Service (MILS)

This project is a lightweight, single-purpose microservice written in Go that provides an immutable, tamper-proof log of transactions. It's designed to be a reliable backend component for auditing, critical event tracking, or any system that requires a high degree of data integrity.

## What Has Been Done

The current implementation includes the following features:

*   **Core Blockchain Logic**: A fully functional blockchain implementation with blocks, transactions, and a Proof-of-Work (PoW) consensus mechanism.
*   **Merkle Tree**: Each block includes a Merkle Tree root to ensure the integrity of its transactions and allow for efficient verification.
*   **Proof-of-Work (PoW) Mining**: New blocks are mined using a PoW algorithm, which secures the ledger and prevents tampering.
*   **Persistence with an Embedded Database**: The blockchain state is persisted using **BoltDB**, an embedded key-value store that runs within the Go application process. For containerized deployments, the database file is stored in a named Docker volume to ensure data persistence across container restarts.
*   **Observability**: The service is instrumented for production-level monitoring:
    *   **Structured Logging**: Uses `logrus` for structured, JSON-formatted logging.
    *   **Prometheus Metrics**: Exposes key metrics on a `/metrics` endpoint, including `block_height`, `mempool_size`, and `mining_time_seconds`.
*   **Containerization**: The entire application stack can be run using Docker and Docker Compose, including the ledger service, Prometheus, and a pre-configured Grafana dashboard.
*   **API Endpoints**: A set of RESTful API endpoints to interact with the ledger:
    *   `POST /transaction`: Add a new transaction to the mempool.
    *   `POST /mine`: Trigger the mining of a new block.
    *   `GET /chain/verify`: Verify the integrity of the entire blockchain.
    *   `GET /block/{index}`: Retrieve a specific block by its index.

## Future Improvements

This project provides a solid foundation, but there are several areas where it could be extended:

*   **Dynamic Difficulty Adjustment**: The PoW difficulty is currently fixed. A future improvement would be to dynamically adjust the difficulty to maintain a consistent block time.
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

2.  **Run the services**:

    ```bash
    docker-compose up --build
    ```

    This will build the ledger service and start the `ledger`, `prometheus`, and `grafana` containers. The ledger's data will be stored in a Docker-managed volume named `ledger-data`.

3.  **Access the services**:

    *   **Ledger API**: `http://localhost:8080`
    *   **Prometheus**: `http://localhost:9090`
    *   **Grafana**: `http://localhost:3000` (a pre-configured dashboard will be available)

### Running Locally

To run the service locally without Docker, you'll need to have Go installed (version 1.18 or later).

1.  **Clone the repository**:

    ```bash
    git clone https://github.com/joseferreira/Immutable-Ledger-Service.git
    cd Immutable-Ledger-Service
    ```

2.  **Install dependencies**:

    ```bash
    go mod download
    ```

3.  **Run the service**:

    ```bash
    go run cmd/ledger/main.go
    ```

    The service will start and listen on port `8080`. A `blockchain.db` file will be created in the project root to store the ledger data.

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

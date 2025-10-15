# Minimalist Immutable Ledger Service (MILS)

This project is a lightweight, single-purpose microservice written in Go that provides an immutable, tamper-proof log of transactions. It operates as a peer-to-peer node in a decentralized network, collectively maintaining a single, consistent blockchain.

## What Has Been Done

The current implementation includes the following features:

* **Robust Peer-to-Peer Mesh Networking**: The service operates as a true P2P node. Nodes connect to a bootstrap peer and then automatically discover and connect to other peers in the network, forming a resilient **mesh topology**. Multiaddress handling has been significantly improved to ensure correct peer identification and connection. Transactions and blocks are gossiped efficiently across all nodes, and the number of connected peers is accurately tracked via Prometheus metrics.
* **Unique Transaction Identification**: Transactions now include a timestamp in their hash, ensuring that even transactions with identical data are uniquely identified. This resolves the issue of only the first transaction being registered.
* **Core Blockchain Logic**: A fully functional blockchain implementation with blocks, transactions, and a Proof-of-Work (PoW) consensus mechanism.
* **Dynamic Difficulty Adjustment**: The PoW difficulty automatically adjusts to maintain a consistent block generation time.
* **Interactive API Documentation**: The API is fully documented using **Swagger/OpenAPI**, providing an interactive UI for exploration and testing.
* **Externalized Configuration**: Key parameters are managed via environment variables and a `.env` file.
* **Persistence**: The blockchain state is persisted using **BoltDB**, an embedded key-value store.
* **Observability**: The service is instrumented for production-level monitoring with structured logging (`logrus`) and **Prometheus** metrics.
* **Containerization**: The entire application stack, including a **3-node decentralized mesh network**, Prometheus, and Grafana, can be run with a single Docker Compose command.

## Future Improvements

This project provides a solid foundation, but there are several areas where it could be extended and polished for a production-ready system:

* **Security: Cryptographic Transaction Signatures**: Implement a robust transaction verification scheme by adding public key `SenderAddress` and `Signature` fields to all transactions. This is a critical addition to ensure **transaction authenticity** and prevent unauthorized ledger manipulation.
* **Advanced Consensus & Synchronization**: Develop a comprehensive strategy for **full chain synchronization**, allowing new nodes to request and validate the entire historical blockchain from peers. Crucially, implement a robust **fork resolution** strategy, such as the **"longest chain wins"** rule, to handle divergent chains and network partitions.
* **Architectural Polish (Dependency Injection)**: Migrate the service initialization in `main.go` to a modern dependency injection framework like **Uber's Fx** for improved testability and cleaner dependency management.
* **Database Migration**: To better align with a microservices architecture, the embedded BoltDB could be replaced with a client-server database like **Redis** for state storage.
* **WebSockets**: For real-time updates, a WebSocket API could be added to notify clients of network events and block finalization.

## Start-Up Guide

### Running the 3-Node Network with Docker (Recommended)

1.  **Clone the repository** and prepare the environment file:

    ```bash
    git clone [https://github.com/joseferreira/Immutable-Ledger-Service.git](https://github.com/joseferreira/Immutable-Ledger-Service.git)
    cd Immutable-Ledger-Service
    cp .env.example .env
    ```

2.  **Generate API Documentation** (only needs to be done once, or after changing API annotations):

    ```bash
    swag init -g cmd/ledger/main.go
    ```

3.  **Run the services**:

    ```bash
    docker-compose up --build
    ```

4.  **Access the services**:

    * **Interactive API Docs (Swagger)**: `http://localhost:8081/swagger/index.html`
    * **Prometheus**: `http://localhost:9090`
    * **Grafana**: `http://localhost:3000`
    * 
## API Documentation

The API is documented using OpenAPI (Swagger). When the application is running, you can access the interactive Swagger UI for detailed information on all endpoints.

*   **URL**: `http://localhost:8081/swagger/index.html` (for Node 1)

For offline viewing, you can see the static specification file here: **[View `swagger.yaml`](./docs/swagger.yaml)**

## Testing the P2P Mesh Network

With your three-node network running, you can test the gossip protocol and see the mesh networking in action.

1.  **Send multiple transactions to Node 2**:

    ```bash
    curl -X POST -H "Content-Type: application/json" -d '{"data": "first transaction from node 2"}' http://localhost:8082/transaction
    curl -X POST -H "Content-Type: application/json" -d '{"data": "second transaction from node 2"}' http://localhost:8082/transaction
    ```

    Check the Docker logs (`docker-compose logs -f ledger-1 ledger-3`). You will see that both Node 1 and Node 3 receive the transactions from Node 2, demonstrating that the nodes have discovered each other and are gossiping correctly.

2.  **Mine a block on Node 2**:

    ```bash
    curl -X POST http://localhost:8082/mine
    ```

    Check the logs for `ledger-1` and `ledger-3`. You will see that both nodes receive the newly mined block from Node 2. This confirms that block propagation works across the mesh network.

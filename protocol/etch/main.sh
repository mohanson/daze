set -ex
curl -X POST http://localhost:8080 \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc": "2.0", "method": "Dns.Wire", "params": ["uygBIAABAAAAAAABBmdvb2dsZQNjb20AABwAAQAAKQTQAAAAAAAA"], "id": 1}'

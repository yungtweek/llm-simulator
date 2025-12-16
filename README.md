# LLM Simulator (System Test Server)

This repository contains a deterministic **LLM behavior simulator** used to validate
an asynchronous, streaming-first LLM architecture (Gateway → Kafka → Worker → UI).

This simulator was used to validate the streaming and failure behavior of the **Talkie** LLM pipeline.
See the full system architecture in the Talkie repository:
https://github.com/tweek/talkie

---

## Why a Simulator

Real LLM APIs are unsuitable for repeatable system tests due to cost, variability,
and limited failure injection. This simulator provides controlled, reproducible
streaming workloads instead.

---

## What Is Validated

### 1. Backpressure & Load Isolation

- Flood Gateway with requests
- Observe Kafka lag growth
- Verify no request loss
- Ensure Gateway stays responsive

**Result:** Kafka acts as a stable backpressure boundary.

---

### 2. Long-Lived Streaming Stability

- Run many concurrent long responses
- Monitor worker memory and event queues
- Verify all streams terminate (`done` or `failed`)

**Result:** No dangling sessions, no memory leaks.

---

### 3. Failure Containment

- Inject mid-stream failures
- Confirm `failed` events are emitted
- Ensure other streams continue unaffected

**Result:** Failures are isolated per request.

---

### 4. End-to-End Latency Decomposition

Collected metrics:

- Gateway ingress time
- Kafka enqueue → dequeue delay
- Worker TTFT / generation / total runtime
- Final stream completion

This allows precise identification of bottlenecks under load.

---

## Summary

This project verifies that a streaming, asynchronous LLM system remains stable,
observable, and predictable under real-world load patterns.

Conclusions are based on **measured behavior**, not assumptions.
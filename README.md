Conductor
===

Abstract
---

This application is intended to provide a level of autonomy during resource 
allocation and their further discovery and management.

The basic pipeline will looks like this:

```mermaid
stateDiagram-v2
    state "Initialize application" as Init
    state "Load previous state" as LoadState

    state "Start processing AMQP" as StartProcessing
    state "Process AMQP requests" as ProcessRequest
    state "Stop processing AMQP" as StopProcessing
    
    state "Estimate connection to Environment" as StartProcessing
    state "Stop processing AMQP" as StartProcessing
    
    state "Start health server" as StartProcessing
    state "Stop health server" as StartProcessing
    
    state "Backtrace changes on pods" as BackTrace
    state "1" as Init
```
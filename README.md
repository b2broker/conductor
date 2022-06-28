Conductor
===

The main purpose of the service is to be a solution for dynamic allocated 
services, track their uniqueness based not only on `image:tag` but also their 
runtime parameters, and count acquisition references.

Modern orchestration environments offers you management of instances or 
instances-collection, as well as, rules-defined scalability. But in more complex 
scenarios, they suggest you to use API. 

This project aims to solves some issues with service-as-a-resource allocation, 
like those where you need to spawn unique instance (custom configured), and help 
you to dispose those when no subscribers left.

Response
---

It should response to discover particular requested instance or instance set, 
spawn new one, — in case once not found, waits for instance's final status, 
report it to requester, track unique, follow the instances changes, count 
subscribers per requested instances and dispose instances which have 0 of them.

### Not responsible for

This project **must not** do, what orchestration platform already do:
- Track failures and force services to recover.

Flow
---

1. Starts
    1. Configure
    2. Connect to orchestration platform 
    3. Read instances from orchestration platform and settle their statuses
    4. Subscribe onto orchestration platform events
    5. Start request server
2. Spawn instance cleanup worker based on ref-counter
3. Serve requests

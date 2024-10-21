# ADR: Account Model Implementation in Platform Mesh using KCP

## Status: Proposed

## Deciders:
-  TBD
- ...

## Date: TBD

## Technical Story:
Evaluate implementation options for the account model in Platform Mesh using KCP (Kubernetes Control Plane) to create a flexible, scalable, and interoperable system for managing service accounts and instances.

## Context and Problem Statement

As part of the ApeiroRA project, we need to implement an account model for the Platform Mesh using KCP. The account model should be simple, scalable, and not locked to regions. It should support service accounts and instances, distinguish between services and applications, and allow for the decoupling of orthogonal aspects such as quotas, service validation, and access control. How can we implement this account model effectively using KCP and the Kubernetes Resource Model (KRM)?

## Decision Drivers

1. Need for a simple and scalable account model
2. Requirement to support hierarchical structures
3. Desire to leverage KCP's workspace model
4. Need for clear distinction between services and applications
5. Requirement for extensibility to accommodate various provider-specific needs
6. Desire for orthogonal aspects to be decoupled from the core account model
7. Requirement to support global user IDs and system-dictated tenant IDs
8. Need for high-level management of accounts and service assignments

## Considered Options

### Option 1: Custom Resource Definition (CRD) for Account Model

This option involves creating a new CRD in KCP to define the account model.

Pros:
- Native Kubernetes approach, easily integrable with KCP
- Allows for declarative management of accounts
- Can be extended using additional fields or annotations
- Facilitates versioning and API evolution

Cons:
- May require frequent updates to the CRD as new requirements emerge
- Could become complex if trying to accommodate all provider-specific needs in one CRD
- Potential performance impact with a large number of custom resources

### Option 2: KCP Workspace as the Core Account Representation

This option uses KCP workspaces as the primary representation of accounts, with additional metadata stored in workspace annotations or labels.

Pros:
- Leverages KCP's existing hierarchical workspace model
- Provides built-in isolation and access control mechanisms
- Allows for easy implementation of hierarchical account structures
- Facilitates management of resources within the context of an account

Cons:
- Limited flexibility in storing complex account data
- May require additional controllers to manage account-specific operations
- Could lead to overloading of workspace concepts

### Option 3: External Database with KCP Integration

This option involves using an external database to store account information, with a thin integration layer in KCP.

Pros:
- Provides flexibility in storing complex account data
- Allows for optimized querying and indexing of account information
- Can easily accommodate provider-specific account details

Cons:
- Introduces additional complexity with external system dependency
- May require additional synchronization mechanisms
- Could potentially impact performance due to external system calls

### Option 4: Hybrid Approach: CRD + KCP Workspace

This option combines a lightweight Account CRD with KCP workspaces, using the CRD for core account data and the workspace for resource management.

Pros:
- Balances flexibility of CRDs with the built-in features of KCP workspaces
- Allows for separation of account metadata from resource management
- Facilitates extension for provider-specific needs through CRD
- Leverages KCP's workspace model for hierarchical structures

Cons:
- Requires careful design to avoid duplication of concepts
- May introduce complexity in managing the relationship between CRD and workspace
- Could potentially lead to inconsistencies if not properly synchronized

## Open Questions and Risks

1. How will the chosen option impact the performance and scalability of the system with thousands of accounts?
2. How can we effectively implement global user IDs and system-dictated tenant IDs in each option?
3. What is the best approach to implement hierarchical account structures in KCP?
4. How can we ensure smooth upgrades and migrations of the account model as requirements evolve?
5. How will each option support the decoupling of orthogonal aspects (quotas, service validation, etc.) from the core account model?


## Decision Outcome

... TBD ...

### Positive Consequences
...

### Negative Consequences
...

## Action Items

1. Develop proof-of-concept implementations for each option, focusing on core account operations.
2. Conduct performance testing with simulated large-scale account data for each option.
3. Consult with the KCP community about best practices for extending workspaces and potential future enhancements.
4. Design and prototype the integration of orthogonal aspects (quotas, access control) with each account model option.
5. Create a detailed mapping of account model requirements to KCP/Kubernetes concepts for each option.
6. Engage with potential service providers to gather feedback on the proposed account model options.

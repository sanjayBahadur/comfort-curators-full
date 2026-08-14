# Jarvis and Hermes Boundaries

## Operating rule

Agents interpret events and produce typed proposals. They are not authorities over operational or financial truth. Application services authenticate the caller, authorize tenant and property scope, validate state, enforce approval limits, execute the transaction, and emit audit plus outbox events.

## Jarvis

Jarvis is a logical property-scoped supervisor, not one process or model instance per property. Its context is assembled from approved property records, active reservations, tickets, stock projections, service scope, policies, relevant owner preferences, and recent summarized history.

Jarvis may:

- identify missing work or conflicts;
- propose turnovers, inspections, restocking, maintenance, incidents, and approvals;
- invoke narrow read tools and proposal commands;
- coordinate Hermes and deterministic dispatch services;
- summarize property health, earnings, and exceptions;
- remember property preferences only after an authorized deterministic write.

Jarvis may not directly mutate storage, approve its own work, spend, change permissions, disclose unrestricted access secrets, file documents, change worker status, or delete evidence.

## Hermes

Hermes is a communication and service-recovery specialist invoked by Jarvis or an explicit staff workflow. It receives the narrow audience, purpose, approved facts, policy, template, language, delivery channel, and review requirement.

Hermes may draft arrival guidance, owner exceptions, issue responses, escalation notices, and recovery follow-up. It does not create conflicting operational work or determine financial liability.

## Typed tool envelope

Every agent request records:

- tenant, property, actor, run, model, prompt-template version, and correlation IDs;
- declared purpose and minimal context references;
- tool name and version;
- structured input and output schema versions;
- policy decision, approval requirement, confidence, timestamps, and outcome;
- redacted error and usage metadata.

Tools are allowlisted per agent. High-risk proposals require human approval. Duplicate proposals are idempotent. Tool timeouts and model failures produce visible operational exceptions, never silent success.

## Model outage

Reservations, tickets, dispatch, access, inventory, approval, billing, and incident flows remain available without any model. During an outage, deterministic rules create required work and humans write communication from approved templates.


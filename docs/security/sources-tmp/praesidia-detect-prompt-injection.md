How to Detect and Defend Against Prompt Injection | Praesidia

Agent Security

# How to Detect and Defend Against Prompt Injection

2026-07-30

        8 min read

Prompt injection is reliably detectable when you apply layered signals — structural pattern matching, content classifiers, behavioral heuristics, and runtime policy — at every point where untrusted text enters an agent's context. No single technique catches everything, but combining them at the right places in the dispatch path makes successful injection substantially harder and easier to audit when it does occur. For the full threat landscape, the threat model: indirect prompt injection post maps the attack surface and realistic attacker paths in detail.

## What prompt injection actually is

A prompt injection attack is any attempt by untrusted text to override or extend an agent's instructions. There are two main forms.

**Direct injection** happens when an attacker controls input sent directly to the agent — a user field, an API parameter, a chat message. The attacker embeds instructions that conflict with the system prompt, such as "ignore previous instructions and instead do X."

**Indirect injection** happens when an agent retrieves content from an external source — a web page, a document, a database record, a tool response — and that content contains adversarial instructions. The agent was never supposed to receive those instructions, but they arrive embedded in what looks like ordinary data. This is the more dangerous form for tool-using agents because the attack surface is every piece of external content the agent touches.

Both forms exploit the same underlying property of large language models: they process instructions and data in the same token stream, so boundary enforcement has to be explicit rather than implicit.

## Why detection is harder than it sounds

The challenge is that legitimate agent inputs and injected instructions often look superficially similar. A user might legitimately ask an agent to "summarize the following document and then send an email" — a multi-step instruction chain. An injected payload embedded in a document might say exactly the same thing, with malicious targets substituted in.

Detection therefore cannot rely on any single syntactic rule. It has to combine signals:

****

****

****

****

****

- Structural patterns: phrases that commonly appear in injection attempts — instruction-override language, authority-claim patterns, and role-reassignment directives
- Role boundary violations: content that attempts to impersonate a system-level speaker or claim elevated authority
- Instruction-in-data context: instructions appearing in positions that should contain only data (tool responses, retrieved documents, API payloads)
- Classifier confidence scores: ML and LLM classifiers that assign a probability score to content being adversarial
- Behavioral anomalies at runtime: an agent that suddenly issues tool calls unrelated to its original task, or targets resources outside the expected scope

No single signal has zero false-positive and zero false-negative rates. The practical approach is to run multiple signals and set an action threshold based on the severity of what the agent is allowed to do.

## Scanning the input layer

The first place to intercept injection is at the input boundary — before the agent sees the content at all.

Pattern-based rules are fast and cheap and catch the most naive attacks. Maintain a list of known injection phrases and structural indicators. Run every incoming message through it. Flag or block matches immediately. Rules alone will miss obfuscated or novel attacks, but they filter the easy cases at near-zero cost.

Content classifiers provide a second signal. These are ML models trained to distinguish adversarial prompts from benign ones. They produce a confidence score that you can threshold — high-confidence matches block, lower-confidence matches log and alert while allowing content to proceed. Classifiers have higher recall than pattern rules but add latency and external dependency to the dispatch path, so they belong on inputs that cross an elevated-risk threshold (content retrieved from external sources, user-supplied free-text in contexts with high-privilege agents).

LLM-based detection is the most expensive option but has the highest semantic understanding. A separate LLM, operating on the input with a tightly controlled system prompt, can be asked to evaluate whether the content attempts to manipulate an agent. This is best reserved for content that has triggered a weaker signal and needs a final adjudication, or for async analysis of content samples after the fact.

## Defending the indirect injection surface

Indirect injection deserves special attention because the attack surface is every external source the agent consults. The defenses operate at the content boundary between external data and the agent's context.

**Source trust levels**: treat content from different sources differently. Content from your own database, verified by an authenticated API call, is lower risk than content fetched from arbitrary URLs. Assign trust tiers and apply heavier scanning to lower-trust content.

**Structural isolation**: when an agent retrieves external content, wrap it in a clear delimiter that signals to the model "this is data, not instruction." Some formats (XML-style tags, quoted blocks) can help the model distinguish context, though this is not a hard boundary — it is a supporting signal, not a primary control.

**Tool response inspection**: every tool call returns a result. That result should pass through the same content scanning pipeline as user input. An injected payload in a tool response is still an injection, and it arrives at exactly the moment the agent is most likely to act on it.

**Output validation**: inspect what the agent produces, not just what it receives. An agent that has been successfully injected will often exhibit the injection in its output — unexpected targets, unusual tool calls, content that does not match the original task. Catching this at the output layer before results are written or actions are taken limits blast radius even when input scanning misses the payload. This pairs with content guardrails for AI agents, which covers the broader framework for defining what agents should and should not produce.

## Layered actions: block, warn, redact, escalate

Detection without response policy is just logging. For each detection signal you need a defined action:

Signal confidence
Typical action

High confidence, safety-critical context
Block: reject the input or suppress the output and return an error

High confidence, lower-risk context
Block or replace with a sanitized version

Medium confidence
Allow with warning logged; human review queue

Low confidence
Log for trend analysis; no immediate action

Any detection in a monitored session
Escalate to a human reviewer

The right thresholds depend on what the agent can actually do. An agent with read-only access to a knowledge base warrants lower thresholds than an agent that can send email, write to a database, or call external APIs. Calibrate action thresholds to the blast radius of a successful injection, not to a universal setting.

## Behavioral monitoring as a secondary defense

Content scanning operates on text. Behavioral monitoring operates on what the agent actually does. The two are complementary.

Signs of a successful injection include: tool calls to resources that are not part of the original task; a sudden change in the subject matter of generated text; attempts to access data outside the expected scope; requests for credentials or sensitive fields that the task did not require; and outputs that closely mirror the structure of an injected payload rather than the original user request.

Behavioral signals are harder to fake than content signals, because they require the attacker to actually cause the agent to perform the target action. Monitoring at this layer gives you a detection opportunity even if the injection payload successfully bypassed content scanning. It also gives you a complete audit trail: which agent, which task, which tool call, which resource, at what time, with what result — the same audit foundation described in how to audit AI agent activity.

## Common questions

**Does a system prompt protect against prompt injection?**

A well-written system prompt helps but does not prevent injection. The model processes all tokens together and the system prompt is a strong prior, not an inviolable boundary. A sufficiently persuasive injection or one that arrives in a position the model treats as authoritative can still shift behavior. System prompts are a useful control layer but they should be combined with input scanning, output inspection, and least-privilege tool scoping rather than relied on alone.

**Should I use a blocklist or an ML classifier?**

Both. Blocklists are fast and have near-zero false-negative rate on known patterns. Classifiers handle novel phrasing and semantic variations that evade rules. Running a blocklist first and a classifier on anything that passes the blocklist (or on higher-risk inputs specifically) gives you coverage across the spectrum at lower total cost than running a classifier on everything. LLM-based detection is best reserved for borderline cases or async review.

**How do I handle indirect injection from third-party tool results?**

Treat every tool result as untrusted input, regardless of the tool's reputation. Scan tool responses through the same content inspection pipeline as user inputs. Apply output validation to catch effects that make it past the input layer. Scope the tools available to each agent so that even a successful injection cannot access capabilities the agent was never supposed to have. Audit logs of tool calls, including the full request and response, give you the reconstruction capability you need after the fact.

**How does least-privilege tool scoping reduce injection risk?**

An agent that can only invoke a narrow set of tools limits what a successful injection can accomplish. Even if an attacker's payload redirects the agent, it cannot exfiltrate data via a tool the agent was never granted, cannot send email if email tooling is not in scope, and cannot write records if the agent only holds read permissions. Least-privilege tool scoping is therefore a defense-in-depth layer that limits blast radius independently of whether detection fires. See how to implement least privilege for AI agents for a practical approach to scoping tool permissions.

---

Praesidia applies content inspection at both the input and output boundaries of every agent task, combining several complementary detection techniques with configurable action thresholds per agent context. For a complete view of the broader threat surface, review the OWASP LLM Top 10 applied to AI agents to understand where prompt injection fits alongside other agent-level risks, and the complete guide to AI agent security for the full defense architecture.

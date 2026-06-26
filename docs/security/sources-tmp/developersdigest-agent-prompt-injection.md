## Official Sources

| Source | Link |
| --- | --- |
| Blue41 Bunq Case Study | How we helped Bunq secure their financial AI assistant |
| OWASP LLM Top 10 | LLM01: Prompt Injection |
| AnthropicPrompt InjectionGuidance | Reducing Prompt Injection Risk |
| OpenAI Safety Best Practices | Safety Best Practices |
| Simon Willison on Prompt Injection | Prompt injection explained |

Security researchers at Blue41 published a case study this week that earned 145 points and 120 comments on Hacker News by June 10, 2026. The subject: they helped Bunq, Europe's second-largest digital bank with over 20 million customers, find and fix an indirect prompt injection vulnerability. The attack required exactly one bank transfer. Cost to the attacker: €0.02.

The vulnerability is not exotic. It is one of the most predictable failure modes in agent architecture, which is precisely why it keeps appearing in production systems. If you are building any agent that reads external data and can take action or produce output, this pattern applies to you directly - whether you run amanaged runtime or your own loop.

Last updated:June 10, 2026

## The Attack Chain, Step by Step

Bunq's banking app includes an AI assistant that lets customers ask natural-language questions about their accounts. When a user asks something like "show me my recent transactions," the assistant fetches the relevant transaction records, places them into the LLMcontext windowas background data, and produces a conversational response.

The problem is in that phrase "places them into the LLM context window." At that point, the transaction records are no longer just data. They are text the model is actively reasoning over - and the model cannot structurally distinguish between "instructions from thesystem prompt" and "data retrieved from the database." It processes both as tokens.

Here is how Blue41 exploited that:

Step 1.The attacker sends a small transfer to the target. SEPA transfer descriptions are free-form text fields. The attacker crafts a description that contains a prompt injection payload - instructions formatted to look authoritative to an LLM. Something that blends into transaction metadata but directs the model to behave differently.

Step 2.The victim opens the banking app and asks the AI assistant any routine question that causes it to fetch recent transactions. They do not need to ask about the specific transfer. Any query that pulls the transaction list is enough.

Step 3.The assistant retrieves the transaction data, which now includes the attacker-controlled description. That text enters the LLM context.

Step 4.The LLM processes the injected instructions. In Blue41's demonstration, the assistant was manipulated into generating a realistic phishing message inside the bank's own interface - a "reauthentication request" that referenced the user's real account context, making it far more credible than any external phishing email.

The authors of the article, tvissers, noted in the HN thread a point worth quoting directly: "The user does not need to ask about the malicious transaction specifically. Any normal question that makes the agent fetch recent transactions could bring the attacker-controlled text into the LLM context."

Co-author tvhamme put the core issue cleanly: "It was never about the prompt, it is about the prompt delivery."

Bunq hadguardrailsin place. They failed because the injected text was crafted to be indistinguishable from normal transaction data when reviewed in isolation. The payload did not use "ignore previous instructions" or other classic patterns. The risk emerged not from any single string but from the interaction between the retrieved data, model behavior, and the assistant's available outputs.

## Why This Generalizes Immediately

The HN commenter globalise83 asked - with knowing sarcasm - whether this also works for customer feedback forms. The answer is yes. The specific attack surface is any string an attacker controls that your agent later reads.

Here is a non-exhaustive list of fields that fit that description:

In every case, the structural problem is the same: attacker-controlled text enters the same context space as the agent's instructions, and the model cannot tell them apart. The delivery mechanism changes. The vulnerability does not.

HN commenter csomar made the conversion argument well: "People are now wary of emails since there is a lot of phishing there. On the other hand, the AI assistant environment could be considered 'safe' by users because it's stuff coming from the bank. So they are more likely to fall for it."

The channel laundering is the upgrade. A phishing message delivered through your own application, by your own AI assistant, referencing real user data, is not a phishing email users have learned to distrust. It is something new.

Get the weekly deep dive

Tutorials on Claude Code, AI agents, and dev tools  -  delivered free every week.

From the archive

[AI Coding Tools Pricing: The June 2026 Reality CheckJun 10, 2026•9 min read](/blog/ai-coding-tools-pricing-june-2026)
[The Pushback on Amodei's Exponential Essay: Too Slow, Too Convenient, or About Right?Jun 10, 2026•9 min read](/blog/amodei-exponential-essay-pushback-roundup)
[Decoding Anthropic's Model Names: Fable, Mythos, and What the Naming Shift SignalsJun 10, 2026•8 min read](/blog/anthropic-model-naming-explained)
[Apache Burr vs LangGraph vs CrewAI: Choosing an AI Agent Framework in 2026Jun 10, 2026•9 min read](/blog/apache-burr-ai-agent-framework-comparison)
## The Defense Checklist

There is no single fix. The correct frame is layers, where each layer reduces the probability and impact of a successful injection. Here is what each layer does - and what it does not do.

### Layer 1: Minimize context to what the task requires

Do not pass fields to the LLM unless the current task requires them. If the user asked "what is my account balance," the transaction description field does not need to enter the context at all. Reduce the injection surface by only including data that is necessary to answer the specific question.

Honest limit:You cannot always know in advance which fields a natural language query will require. Semantic routing can help but adds its own complexity.

### Layer 2: Treat all retrieved content as data, not instructions

Use structural separation: wrap retrieved data in explicit XML-style tags or delimiters, instruct thesystem promptto treat everything inside them as user-provided data regardless of how it is formatted, and have the model reference it rather than follow it. For example:

```
<retrieved-data source="transactions" trust="untrusted">
{{ transaction_records }}
</retrieved-data>
```

Some models respect this framing better than others. It is not a hard boundary - it is a hint that shifts probability. A well-crafted payload can still escape it.

Honest limit:This is defense-in-depth, not a guarantee. The HN commenter crote made the SQL injection analogy correctly: "You're still just one clever prompt away from getting pwned. It's like trying to solve SQL injection by attempting to use an ever-increasing pile of regexes for input validation, rather than just getting rid of string concatenation and using prepared statements instead." The prepared-statement equivalent for LLMs does not yet exist.

### Layer 3: Constrain what the agent can output and do

The Bunq attack succeeded in producing a phishing link inside the bank's interface. An output allowlist would have stopped that specific outcome: if the agent is not permitted to generate external URLs, the payload cannot exfiltrate users to attacker-controlled sites. Similarly, if the agent cannot initiate outbound transfers, injected transfer instructions have no execution path.

Hocuspocus in the HN thread put it directly: "A chatbot should absolutely not be able to display arbitrary and clickable links outside a pretty tight whitelist (like, the bank FAQ)."

Honest limit:Constraining outputs stops many attack outcomes but not all. An agent that can only generate text can still be manipulated into producing misleading information, suppressing real information, or steering user behavior without any external links.

### Layer 4: Human confirmation for high-impact actions

Any action with real-world consequences - sending money, sending messages, updating records, triggering workflows - should require explicit human confirmation before execution. The confirmation prompt should display the full proposed action in plain language, not the LLM's summary of it.

Honest limit:Confirmation UX can itself be manipulated. If the injected payload causes the agent to present a misleading confirmation ("Confirm transfer to savings account" when the destination is attacker-controlled), users may confirm without noticing. The confirmation step needs to display system-derived values, not LLM-generated summaries.

### Layer 5: Runtime behavioral monitoring

HN commenter bilekas suggested isolating the AI to a specific API with no access that makesprompt injectionactionable. That is the right instinct - least-privilege access at the tool level. But Blue41 added a layer beyond prevention: monitoring what the agent actually does at runtime, building behavioral profiles of normal operation, and flagging deviations.

When an assistant is compromised, its behavior changes in ways that are often observable: it starts generating URLs it normally does not produce, it accesses data sources outside its usual pattern, it calls tools in unusual sequences. A detection layer watching those signals can catch injections that prevention failed to stop.

Honest limit:Behavioral baselines require time to establish and generate false positives. This is a detection layer, not a prevention layer.

## Defense Layers at a Glance

| Layer | What It Stops | What It Does Not Stop |
| --- | --- | --- |
| Context minimization | Injections in fields not retrieved | Injections in fields the task legitimately needs |
| Data/instruction separation (tagging) | Naive payloads; reduces injection probability | Well-crafted payloads that exploit model ambiguity |
| Output and action allowlists | Link exfiltration; unauthorized tool calls | Text manipulation; information suppression |
| Human confirmation for side effects | Automated execution of injected commands | Confirmed execution if confirmation UI is also manipulated |
| Least-privilege tool access | Injections that require capabilities the agent lacks | Read-only data manipulation and user deception |
| Runtime behavioral monitoring | Post-hoc detection; limits blast radius | Does not prevent the initial compromise |

No single row in that table is sufficient. The practical goal is to make each step of an attack chain require bypassing a separate control.

## What This Means for Builders Right Now

The fn-mote comment on HN captures where we are with LLM security maturity: "We're not even at the 'ASLR' level of protection for LLMs yet." That is an accurate and sobering benchmark. Memory randomization in operating systems was a partial mitigation introduced decades after buffer overflows were understood. We are earlier than that with prompt injection.

That does not mean the problem is unsolvable in your system. It means you need to design for the assumption that the LLM will sometimes be influenced by injected content, and ask: what is the worst outcome if that happens, and what structural controls limit that outcome?

The Bunq case is a useful test. Ask yourself: if an attacker placed arbitrary text into every string your agent reads, what is the worst outcome? Can they initiate financial transactions? Can they send messages on behalf of your system? Can they exfiltrate user data? Can they present misleading information in a high-trust context?

The answers tell you where your hardening priorities are.

If your agent can take side-effect actions, start there. Lock down what it can do before worrying about whether you can filter every possible injection payload. You cannot. But you can ensure that what gets injected has nowhere useful to go.

## FAQ

### What is indirect prompt injection in AI agents?

Indirect prompt injection is when an attacker embeds instructions inside data that anAI agentlater retrieves and processes - rather than entering instructions directly through the user interface. The attacker does not interact with the agent directly. They control text in a data source (a transaction description, an email, a document) that the agent reads as part of its normal operation. When the agent processes that data, it may follow the embedded instructions as if they came from the system prompt.

### How is this different from a standard prompt injection attack?

In standard (direct) prompt injection, the attacker interacts with the agent directly, entering instructions through the chat interface or API. Indirect injection is more dangerous in practice because the attacker does not need any access to the target system at all. They just need to place text somewhere the agent will eventually retrieve - a public document, an email sent to the victim, a payment description. The attack happens asynchronously and can be targeted at many users at once.

### Can input filtering or guardrails prevent prompt injection in banking agents?

Partially. Input filters and classifiers can catch naive, obvious payloads. The Bunq case demonstrated the limit: the malicious payload was crafted to look like ordinary transaction metadata when reviewed in isolation. The danger only emerged when the agent combined it with real account context and generated a response. Static text classification alone does not see that emergent risk. Blue41's conclusion is thatguardrailsneed to be one layer in a defense-in-depth model, not the primary control.

### What is the safest architecture for an AI agent that reads user-controlled data?

The safest architecture assumes injection will sometimes succeed and limits the blast radius. Practically: strip retrieved data to only what the current task requires; use structural tagging to signal data versus instructions; restrict output types to a narrow allowlist appropriate to the task; gate any side-effect action (money movement, message sending, record mutation) behind explicit human confirmation that displays system-derived values rather than LLM-generated summaries; and enforce least-privilege at the tool level so the agent cannot call capabilities it does not need. Combine that with runtime behavioral monitoring to detect anomalies when preventive controls are bypassed.

## Sources

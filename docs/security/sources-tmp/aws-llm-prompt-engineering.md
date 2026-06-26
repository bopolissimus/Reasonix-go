# Source: https://docs.aws.amazon.com/prescriptive-guidance/latest/llm-prompt-engineering-best-practices/best-practices.html

View a markdown version of this page

# Best practices to avoid prompt injection attacks

The following guardrails and best practices were tested on a RAG application that was
        powered by Anthropic Claude as a demonstrative model. The suggestions are highly applicable
        to the Claude family of models but are also transferrable to other non-Claude LLMs, pending
        model-specific modifications (such as removal of XML tags and using different dialogue
        attribution tags).

## Use <thinking> and <answer> tags

A useful addition to basic RAG templates are<thinking>and<answer>tags.<thinking>tags enable the
            model to show its work and present any relevant excerpts.<answer>tags contain the response to be returned to the user. Empirically, using these two tags
            results in improved accuracy when the model answers complex and nuanced questions that
            require piecing together multiple sources of information.

## Use guardrails

Securing an LLM-powered application requires specific guardrails to acknowledge and
            help defend against thecommon attacksthat were
            described previously. When we designed the security guardrails in this guide, our
            approach was to produce the most benefit with the fewest number of tokens introduced to
            the template. Because a majority of model vendors charge by input token, guardrails that
            have fewer tokens are cost-efficient. Additionally, over-engineered templates have been
            shown to reduce accuracy.

### Wrap instructions in a single pair of salted sequence tags

Some LLMs follow a template structure where information is wrapped in XML tags to
                help guide the LLM to certain resources such as conversation history or documents
                retrieved. Tag spoofing attacks try to take advantage of this structure by wrapping
                their malicious instructions in common tags, and leading the model into believing
                that the instruction was part of its original template.Salted
                    tagsstop tag spoofing by appending a session-specific alphanumeric
                sequence to each XML tags in the form<tagname-abcde12345>. An
                additional instruction commands the LLM to only consider instructions that are
                within these tags.

One issue with this approach is that if the model uses tags in its answer, either
                expectedly or unexpectedly, the salted sequence is also appended to the returned
                tag. Now that the user knows this session-specific sequence, they can accomplish tag
                spoofing—possibly with higher efficacy because of the instruction that
                commands the LLM to consider the salt-tagged instructions. To bypass this risk, we
                wrap all the instructions in a single tagged section in the template, and use a tag
                that consists only of the salted sequence (for example,<abcde12345>). We can then instruct the model to only
                consider instructions in this tagged session. We found that this approach stopped
                the model from revealing its salted sequence and helped defend against tag spoofing
                and other attacks that introduce or attempt to augment template instructions.

### Teach the LLM to detect attacks by providing specific instructions

We also include a set of instructions that explain common attack patterns, to
                teach the LLM how to detect attacks. The instructions focus on the user input query.
                They instruct the LLM to identify the presence of key attack patterns and return
                "Prompt Attack Detected" if it discovers a pattern. The presence of these
                instructions enable us to give the LLM a shortcut for dealing with common attacks.
                This shortcut is relevant when the template uses<thinking>and<answer>tags, because the LLM usually parses malicious
                instructions repetitively and in excessive detail, which can ultimately lead to
                compliance (as demonstrated in the comparisons in the next section).

Thanks for letting us know we're doing a good job!

If you've got a moment, please tell us what we did right so we can do more of it.

Thanks for letting us know this page needs work. We're sorry we let you down.

If you've got a moment, please tell us how we can make the documentation better.
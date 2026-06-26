# Apply tags to user input to filter content

Input tags allow you to mark specific content within the input text that you want to
be processed by guardrails. This is useful when you want to apply guardrails to certain
parts of the input, while leaving other parts unprocessed.

###### Important

Input tagging with XML tags applies only to theInvokeModelandInvokeModelWithResponseStreamAPIs. If you are using theConverseAPI, use theguardrailConfigurationfield in the content blocks to control which parts of your messages are evaluated by guardrails. For more information, seeInclude a guardrail with the Converse API.

For example, the input prompt in RAG applications may contain system prompts, search
results from trusted documentation sources, and user queries. As system prompts are
provided by the developer and search results are from trusted sources, you may just need
the guardrails evaluation only on the user queries.

In another example, the input prompt in conversational applications may contain system
prompts, conversation history, and the current user input. System prompts are developer
specific instructions, and conversation history contain historical user input and model
responses that may have already been evaluated by guardrails. For such a scenario, you
may only want to evaluate the current user input.

By using input tags, you can better control which parts of the input prompt should be
processed and evaluated by guardrails, ensuring that your safeguards are customized to
your use cases. This also helps in improving performance, and reducing costs, as you
have the flexibility to evaluate a relatively shorter and relevant section of the input,
instead of the entire input prompt.

Tag content for guardrails

To tag content for guardrails to process, use the XML tag that is a combination of a
reserved prefix and a customtagSuffix. For example:

```
{
"text": """
You are a helpful assistant.
Here is some information about my account:
- There are 10,543 objects in an S3 bucket.
- There are no active EC2 instances.
Based on the above, answer the following question:
Question:
<amazon-bedrock-guardrails-guardContent_xyz>
How many objects do I have in my S3 bucket?
</amazon-bedrock-guardrails-guardContent_xyz>
...
Here are other user queries:
<amazon-bedrock-guardrails-guardContent_xyz>
How do I download files from my S3 bucket?
</amazon-bedrock-guardrails-guardContent_xyz>
""",
"amazon-bedrock-guardrailConfig": {
"tagSuffix": "xyz"
}
}
```

In the preceding example, the content`How many objects do I have in my  S3
bucket?`and ""How do I download files from my S3
bucket?" is tagged for guardrails processing using the tag<amazon-bedrock-guardrails-guardContent_xyz>. Note that the prefixamazon-bedrock-guardrails-guardContentis reserved by
guardrails.

Tag Suffix

The tag suffix (xyzin the preceding example) is a dynamic value that
you must provide in thetagSuffixfield inamazon-bedrock-guardrailConfigto use input tagging. It is
recommended to use a new, random string as thetagSuffixfor every
request. This helps mitigate potential prompt injection attacks by making the tag
structure unpredictable. A static tag can result in a malicious user closing the XML
tag and appending malicious content after the tag closure, resulting in aninjection attack. You are limited to alphanumeric
characters with a length between 1 and 20 characters, inclusive. With the example
suffixxyz, you must enclose all the content to be guarded using the
XML tags with your suffix:<amazon-bedrock-guardrails-guardContent_xyz>your
content</amazon-bedrock-guardrails-guardContent_xyz>.
We recommend that you use a dynamic unique identifier for each request as a tag
suffix.

Multiple tags

You can use the same tag structure multiple times in the input text to mark different
parts of the content for guardrails processing. Nesting of tags is not allowed.

Untagged content

Content outside of input tags isn't processed by guardrails. This allows you to
include instructions, sample conversations, knowledge bases, or other content that
you deem safe and don't want to be processed by guardrails. If there are no tags in
the input prompt, the complete prompt will be processed by guardrails. The only
exception isDetect prompt attacks with Amazon Bedrock Guardrailsfilters, which require input tags to be present.

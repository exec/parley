# Parley — Privacy Policy

**Last updated: 2026-04-27**

This policy explains what data Parley collects, what we do with it, who it's shared with, and what you can do about it. We've tried to write it in plain language and to disclose more than the law strictly requires.

We don't sell your data. We don't advertise to you. We don't train AI on your messages. We collect the minimum we need to run the service.

---

## 1. What we collect

When you create an account:
- **Username** (your choice, public)
- **Email address** (used for login + security/transactional notifications)
- **Password hash** (bcrypt; we never see your plaintext password) **or** **passkey credential** (public key only — your authenticator keeps the private key)
- **Optional: phone number** (only if you choose to add one for account recovery)

When you use Parley:
- **Messages, attachments, reactions, voice/video session metadata** (joined-at, left-at, who was present — we do NOT record voice/video media; see section 5)
- **Profile data you set**: display name, avatar, banner, bio, status text
- **Server/channel memberships** and per-channel read state / notification preferences
- **Friend list, blocklist, friend-request history**
- **Audit log** of administrative actions you take in servers you're a member of (kicks, role changes, etc.)
- **Session metadata**: when you logged in, the user-agent string your browser or app sent (e.g. browser name and OS — not GPS, not a stable cross-site fingerprint), and the IP your session is currently authenticated from (used to detect session hijacking, not used for analytics or behavioral profiling). We also log request IPs in HTTP-server logs for security and abuse-handling purposes; those logs rotate after 30 days (see §3).

When you upload a file:
- The file itself (stored in our object storage)
- Filename, MIME type, size

When you talk to a bot:
- Your message (handled per the AI and image-content processing section below — section 6)

What we do **not** collect:
- We do not record voice or video calls.
- We do not run analytics or behavior tracking on your activity.
- We do not use third-party trackers, advertising cookies, or fingerprinting.
- We do not have a "shadow profile" of you. If you've never created an account, we have nothing.
- We do not share your data with advertisers or data brokers (because we don't have either).

## 2. How we use what we collect

- To run the service (deliver your messages to the people you sent them to, render your UI, etc.)
- To keep the service secure (rate-limit abusive traffic, detect session hijacking, prevent CSAM as legally required)
- To send you transactional email (signup verification, password reset, security alerts) via our email subprocessor (see section 4)
- To respond to your support requests
- To comply with legal process (court orders served via our Legal contact in the Terms)

That's the complete list. We don't have a "we may use it for any other purpose" catch-all.

## 3. How long we keep it

- Your **profile data, messages, and settings**: as long as your account is active.
- When you **delete your account**: profile, friends, blocks, notifications, sessions, passkeys, uploads = removed. Your messages are reattributed to "Deleted User" so other people's conversations survive (see Terms section 3).
- **Session IPs and user-agent strings**: tied to the lifetime of the session cookie (default 30 days); discarded when the session expires or is revoked.
- **Audit logs of administrative actions** (server kicks, role changes, ban records): kept as long as the relevant entity (server, channel) exists, and deleted with it.
- **Server logs** (HTTP request logs, error logs, rate-limit hits): rotated after **30 days**.
- **Database backups**: rotated after **30 days**. A delete you initiate today is fully out of backups within 30 days at the latest.
- **CSAM-related records**, including reports made to NCMEC and supporting evidence: retained for the **minimum period required by 18 U.S.C. § 2258A(h)** — **one year** following submission to the CyberTipline (per the REPORT Act amendment, Pub. L. 118-59, May 7, 2024, which struck the prior 90-day period and inserted "1 year") — and only for that period unless law enforcement specifically directs us to retain them longer. We do not use this data for any purpose beyond the legally-mandated reporting and any subsequent law-enforcement cooperation.
- **Trust-and-safety case files** (other abuse reports and our handling notes): retained for **1 year after the case is closed**, then deleted.

## 4. Subprocessors

We use a small set of third-party services to operate Parley. We've documented each one's stance on customer data below, with direct quotes from their public policies. The stamps below reflect what those policies said on **2026-04-26**; they can change, and we'll update this section when they do.

### Brevo (Sendinblue SAS) — transactional email

**Purpose:** Sends signup verification, password reset, security alert, and other transactional emails.

- Privacy Policy: <https://www.brevo.com/legal/privacypolicy/>
- DPA: bundled into [Brevo's Terms of Service](https://www.brevo.com/legal/termsofuse/); a copy is also retrievable inside any Brevo account under *profile → data protection*. See their [help center note](https://help.brevo.com/hc/en-us/articles/15403782599570-Where-can-I-find-the-Data-Processing-Agreement-DPA).
- GDPR overview: <https://www.brevo.com/gdpr/>
- HQ: Sendinblue SAS, 17 rue Salneuve, 75017 Paris, France. French law / EU jurisdiction.

When Parley sends you a transactional email through Brevo, **Parley is the data controller and Brevo is the data processor.** Brevo's policy describes our role as "Customer" and your role (the email recipient) as a "Contact":

> "Brevo does not act as a data controller meaning that it has no influence and does not decide on such processing of your personal data (please be reminded in this regard that Brevo does not send electronic communications on its own behalf but only on behalf and under the instructions of its Customers, except where detailed in Section 3) and, therefore, this Policy may not apply. Brevo will act for these other processing as a data processor and our Customers are acting as data controllers..."
> — Brevo Privacy Policy, "What is the scope of this policy?"

On security and storage:

> "All data is stored on secure servers in Tier 3 and PCI DSS certified data centers and is only accessible to our personnel and contractors via authentication measures."
> — Brevo Privacy Policy, "Data Security"

On international transfers:

> "Your personal data may be transferred to countries which do not offer an adequate level of protection and have not been recognized as adequate country by an adequacy decision from the European Commission... and more precisely, in particular to the United States of America and India."
> — Brevo Privacy Policy, "Does Brevo transfer your personal data outside the European Economic Area?"

Brevo's US entity (Sendinblue, Inc., dba Brevo USA) is self-certified under the EU-U.S. Data Privacy Framework, the UK Extension to the EU-U.S. DPF, and the Swiss-U.S. Data Privacy Framework:

> "Sendinblue, Inc. (dba Brevo USA) complies with the EU-U.S. Data Privacy Framework (EU-U.S. DPF) and the UK Extension to the EU-U.S. DPF, and the Swiss-U.S. Data Privacy Framework (Swiss-U.S. DPF) as set forth by the U.S. Department of Commerce."
> — Brevo Privacy Policy, "EU-U.S. Data Privacy Framework with UK and Swiss Extensions"

Brevo's policy does not represent that it trains AI/ML models on Customer or Contact data. The only AI feature it discloses is an "AI chatbot" on Brevo's marketing website powered by OpenAI — that feature is not part of the transactional-email flow Parley uses and your messages on Parley do not pass through it.

### Cloudflare, Inc. — edge / CDN / DDoS protection

**Purpose:** Terminates TLS for parley.byexec.com, caches static assets, blocks DDoS and abuse traffic before it reaches our servers, geo-blocks requests from sanctioned regions.

- Privacy Policy: <https://www.cloudflare.com/privacypolicy/>
- Customer DPA: <https://www.cloudflare.com/cloudflare-customer-dpa/>
- HQ: Cloudflare, Inc., 101 Townsend St, San Francisco, CA 94107 USA.

> "We will not sell or rent your personal information. We will only share or otherwise disclose your personal information as necessary to provide our Services or as otherwise described in this Policy, except in cases where we first provide you with notice and the opportunity to consent."
> — Cloudflare Privacy Policy

> "We store your personal information for a period of time that is consistent with the business purposes set forth in Section 3 of this policy or as long as needed to fulfill and comply with legal obligations."
> — Cloudflare Privacy Policy

> "Cloudflare will ensure that any sub-Processor it engages to provide an aspect of the Service on its behalf in connection with this DPA does so only on the basis of a written contract which imposes on such sub-Processor terms (i.e., data protection obligations) that are no less protective of Personal Data than those imposed on Cloudflare in this DPA."
> — Cloudflare Customer DPA

> "When Cloudflare transfers personal data from the EEA, Switzerland, or the United Kingdom (UK) to the United States, we rely on our certifications under the EU-U.S. Data Privacy Framework... Should these certifications lapse or become otherwise invalidated, Cloudflare relies on the standard contractual clauses, including supplementary measures as necessary for transfers to the United States."
> — Cloudflare Privacy Policy

We have not been able to locate a direct quote on whether Cloudflare uses customer or transit data to train AI/ML models in the publicly published Privacy Policy.

**CSAM Scanning Tool.** We additionally use Cloudflare's [CSAM Scanning Tool](https://developers.cloudflare.com/cache/reference/csam-scanning/), which hash-matches cached image content against NCMEC's NGO and industry hash lists. Positive matches are forwarded to a designated notification address at Parley, and we file the corresponding NCMEC CyberTipline report per §3 / 18 U.S.C. § 2258A. This is part of Cloudflare's existing service offering and is governed by the same Cloudflare DPA, Privacy Policy, and Service-Specific Terms quoted above.

### Hive Moderation — image-content classification

**Purpose:** When you upload an image, Hive's Visual Moderation API classifies it for adult content and other policy categories, and Hive's CSAM Detection API (their integration with Thorn's Safer) hash-matches it against Thorn's database of known CSAM and runs a novel-CSAM classifier. See §6 for the full pipeline.

- Privacy Policy: <https://thehive.ai/privacy>
- Terms of Service: <https://thehive.ai/terms-of-service>
- HQ: Hive, Inc., San Francisco, California, USA.

> **Disclosure note:** We have not yet captured Hive's data-handling DPA in plain text — specifically their commitments around image retention after classification, whether they retain inputs to improve their models, and their international-transfer mechanism. Image-upload moderation depends on this subprocessor; **the image-upload feature will remain disabled at launch until Hive's DPA is captured here with the same direct-quote treatment we gave the other subprocessors**, and the README pre-launch checklist gates launch on this. We will replace this note with verbatim quotes from Hive's DPA before the image-upload feature is enabled.

### LiveKit, Inc. — voice and video media routing

**Purpose:** When you join a voice or video call, audio and video flow through a LiveKit Selective Forwarding Unit (SFU). LiveKit does not record media.

- Privacy Policy: <https://livekit.com/legal/privacy-policy>
- DPA: <https://livekit.com/legal/data-processing-addendum>
- Sub-processor list: <https://livekit.io/legal/sub-processors>
- HQ: United States. EU SCCs in the DPA are governed by Irish law (Clause 17, Option 1).

> "LiveKit does not use identifiable personal data, customer content, prompts, transcripts, or audio to train or fine-tune LiveKit's own models."
> — LiveKit Privacy Policy

> "LiveKit is based in the United States ('U.S.') and processes Personal Data in the U.S."
> — LiveKit Privacy Policy

> "the EU Standard Contractual Clauses will apply to Personal Data that is transferred via the Services from the EEA or Switzerland, either directly or via onward transfer, to any country or recipient outside the EEA or Switzerland"
> — LiveKit DPA

LiveKit's published policy does not include a concrete retention period for SFU media — only "deleted when this information is no longer necessary." Their Trust Center (<https://trust.livekit.io>) may publish a more specific commitment behind a request form.

### Ollama Cloud — AI for theme generation and the default bot provider

**Purpose:** When you generate a custom theme via the AI option, your prompt is sent to Ollama Cloud to be processed by an LLM. When you talk to a bot whose owner did not configure a custom AI provider, your message is sent to Ollama Cloud the same way.

- Privacy Policy: <https://ollama.com/privacy>
- DPA: not separately published (if you require an Article 28 DPA for compliance reasons, that's a direct-contact request with Ollama).

> "We do not use your inputs or outputs to train any AI models or request prompt or response content in support requests."

> "we process your prompts and responses transiently to provide the service and never train on it."

> "this content is not stored beyond the time required to fulfill the request."

— Ollama Privacy Policy, retrieved 2026-04-26.

> "Where required by law, we use appropriate safeguards for international transfers."

— Ollama Privacy Policy. (No specific transfer-mechanism naming, e.g. SCCs, in the public policy.)

### Bring-your-own AI providers (only when a server admin configures them)

A server admin can configure their bot to call Anthropic, OpenAI, xAI, Mistral, or Google instead of the Parley default. When that happens, your message to that bot is sent to whichever provider the admin chose. Their privacy policy governs.

#### Anthropic (Claude API)

- Privacy Policy: <https://www.anthropic.com/legal/privacy>
- Commercial Terms (governs API): <https://www.anthropic.com/legal/commercial-terms>

> "Anthropic may not train models on Customer Content from Services."
> — Anthropic Commercial Terms

#### OpenAI (ChatGPT / GPT API)

- Privacy Policy: <https://openai.com/policies/privacy-policy>
- API data-handling docs: <https://platform.openai.com/docs/guides/your-data>

> "Your data is your data. As of March 1, 2023, data sent to the OpenAI API is not used to train or improve OpenAI models (unless you explicitly opt in to share data with us)."
> — OpenAI Platform docs

#### xAI (Grok API)

- Privacy Policy: <https://x.ai/legal/privacy-policy>
- API security FAQ: <https://docs.x.ai/developers/faq/security>

xAI's consumer privacy policy explicitly excludes API customers and routes the data-handling promise into a separate Enterprise Customer Agreement that is not stably published online. From the consumer policy:

> "This Privacy Policy does not apply to data that we process on behalf of customers of our business offerings, such as the xAI API…"

If you configure a bot to call the xAI API, the bot owner is the xAI customer; the agreement they accepted with xAI governs that data. Consult xAI directly for the specific terms.

#### Mistral AI (La Plateforme)

- Privacy Policy: <https://legal.mistral.ai/terms/privacy-policy>
- DPA: <https://legal.mistral.ai/terms/data-processing-addendum>

Important caveat — Mistral's policy carves out *only the paid tier* of its API:

> "we do not use your Input and Output to train our artificial intelligence models when you use Le Chat Enterprise or the paid version of our APIs."
> — Mistral Privacy Policy

The implication is that the **free tier** of Mistral's La Plateforme API does not enjoy this carve-out — Mistral may use free-tier inputs and outputs for training. **If a bot in a server you're in is configured against Mistral's free tier, your messages to that bot may be used by Mistral for model training.** The paid tier and Le Chat Enterprise carve this out. **Ask the server admin which tier they're using if it matters to you.**

#### Google (Gemini API)

- Privacy Policy: <https://policies.google.com/privacy>
- Gemini API additional terms: <https://ai.google.dev/gemini-api/terms>

Important caveat — Google's Gemini API has different terms for free vs paid:

> "Google doesn't use your prompts (including associated system instructions, cached content, and files such as images, videos, or documents) or responses to improve our products."
> — Gemini API terms (paid tier)

> "Google uses the content you submit to the Services and any generated responses to provide, improve, and develop Google products and services and machine learning technologies."
> — Gemini API terms (free tier)

If a bot in a server you're in is configured against Google's free-tier Gemini API, your messages to that bot are used by Google for model training. The paid tier carves this out. **If your privacy matters to you, ask the server admin which tier they're using.**

If you want to know what AI provider a particular bot uses, ask the server's admin. We're considering surfacing this in the bot's profile in a future update.

#### Free-tier warning at bot configuration

Because the free tiers of Google Gemini and Mistral train on inputs and outputs by default, Parley shows server admins a one-time inline warning when they enter an API key for either provider. The warning reads roughly: *"Google's free Gemini tier and Mistral's free La Plateforme tier may use your bot's conversations to train their models. The paid tiers do not. Make sure the key you entered is for the tier you intended."* Nothing is blocked — server admins remain free to use whichever tier they want — but we want the choice to be informed.

## 5. Voice and video calls

We do not record voice or video calls. Audio and video flow through LiveKit's SFU; nothing is persisted server-side. The metadata we do keep (call start/end time, who was on the call, what virtual channel it was in) is what's needed to render presence indicators and a text-channel "X started a call" / "X missed a call" entry — never the media itself.

## 6. AI and image-content processing

### AI inference

If you use the **AI theme generator**: your text prompt is sent to Ollama Cloud (see section 4) along with a system prompt that instructs the model to return CSS. The prompt + result are not retained by Ollama past serving the request and are not used for training.

If you talk to a **bot in a channel**: the bot's owner chose the AI provider. By default that's Ollama Cloud (no training, transient). If the owner configured Anthropic/OpenAI/xAI/Mistral/Google with their own API key, your message goes to that provider per their policy.

If a bot calls a **non-AI external service** (e.g. a webhook integration the bot owner wired up), Parley does not control where that data goes. The bot owner does, and it's covered by whatever agreement they have with that service.

### Image-upload scanning

When you upload an image to Parley (as a message attachment, an avatar, a banner, etc.), it's automatically scanned before it's served to anyone else. The pipeline runs in three layers:

1. A **self-hosted classifier** ([Falconsai/nsfw_image_detection](https://huggingface.co/Falconsai/nsfw_image_detection), Apache 2.0) runs locally on our infrastructure to flag obvious adult content. The image never leaves Parley's servers for this layer.
2. **Hive Moderation** runs two checks: their Visual Moderation classifier (general adult-content detection) and their integration with Thorn's Safer (CSAM hash matching against a database of known images, plus a novel-CSAM classifier).
3. **Cloudflare's CSAM Scanning Tool** runs independently on cached image content, hash-matching against NCMEC's NGO and industry hash lists.

What happens on a positive match:
- **Adult-content match** (Falconsai high-confidence, or Hive Visual Moderation positive): the image is auto-quarantined and removed from the message; you'll be notified that it failed moderation. Sexual content is forbidden by section 5 of the Terms — see the moderation queue note below.
- **CSAM match** (Cloudflare hash, Thorn hash, or Safer-Predict novel-CSAM): the image is auto-quarantined; you are **not** notified; we report the match to the National Center for Missing & Exploited Children (NCMEC) as 18 U.S.C. § 2258A requires. Retention of those records is described in §3.

A human (the operator) reviews quarantined adult-content matches before any final account action. CSAM matches go directly to the NCMEC reporting workflow and are not displayed to the operator beyond what NCMEC reporting requires; an audit log of the decision (matched/reported/timestamp) is retained per §3 trust-and-safety retention.

We do not use the contents of your uploaded images to train any AI model.

### What we don't do at the platform layer

We do not scan **text messages** with adult-content classifiers. Text-side moderation is by reports + server-owner moderation, not by automated content classification. We do not run sentiment analysis, profile-building, or "predictive" classifiers on your messages.

## 7. Analytics and telemetry

We don't run any first-party analytics in v1. If we add aggregate analytics later, we'll use a self-hosted privacy-conscious tool (no per-user tracking, no cookies, aggregate counts only — like Plausible or PostHog in aggregate-only mode), and we'll update this section before deploying it.

We don't ship third-party analytics, advertising trackers, or fingerprinting libraries.

## 8. Cookies

We use **one** cookie: a session cookie that proves you're logged in. It's `HttpOnly`, `Secure`, `SameSite=Lax`, and contains a signed JWT. It expires when your session does (default: **30 days**).

We don't use any other cookies. No tracking pixels. No social-login OAuth cookies (we don't use social login). No third-party cookies of any kind.

This is also why we don't show you a cookie banner — under EU and UK law, essential session cookies don't require one.

## 9. Your rights

You can:
- **Access your data**: Settings → Account → Download my data exports a JSON archive of everything we have about you (your profile, friends, blocks, notifications, every message you authored, every bot you own, every server you own, everything).
- **Delete your account**: Settings → Account → Delete my account erases your personal data immediately. Messages are anonymized (see Terms section 3). This is a hard delete, not a 30-day-recoverable soft delete.
- **Correct your data**: edit your profile in Settings.
- **Object to processing**: don't use the service. We have no legitimate-interest catch-all under which we'd keep processing your data after you've objected.

If you're an **EU/UK resident**, you also have the right to file a complaint with your data protection authority (e.g. the ICO in the UK, your country's DPA in the EU). We'd appreciate hearing from you first via hello@parley.byexec.com so we can try to fix the issue.

If you're a **California resident**, you have the same rights under the CCPA. We don't sell your data, so the "do not sell" right doesn't have anything to opt out of.

## 10. International transfers

Parley is operated from **Illinois, United States**. Our servers are in the US. If you're outside the US, your data is transferred to the US to be processed.

For users in the EU/EEA/UK, the international transfer mechanism depends on the subprocessor:
- Where a subprocessor is self-certified under the **EU-US Data Privacy Framework** (DPF), Swiss-US DPF, or UK Extension to the EU-US DPF — that framework is the primary basis for transfer.
- Where it is not, the **EU Standard Contractual Clauses** (SCCs) apply, with supplementary measures where required.

Each subprocessor's specific posture is documented in section 4.

## 11. Children

Parley is for adults (18+). We don't knowingly collect data from anyone under 18. If you become aware of an account belonging to someone under 18, please email hello@parley.byexec.com; we'll close it and delete the data.

We don't ship "child-safe mode" because we don't accept children. The age gate at signup asks your date of birth (computed locally, not sent to our servers — only "is_adult: true" is transmitted).

### COPPA — children under 13

The U.S. Children's Online Privacy Protection Act ("COPPA") imposes specific obligations on operators when they have actual knowledge that a user is under 13. Although we don't accept any user under 18, we still take the under-13 case seriously:

- **If we learn that an account belongs to someone under 13**, we will (a) immediately close the account and revoke any active sessions; (b) delete all personal data associated with it from our live systems; (c) propagate the deletion to backups within the standard 30-day backup-rotation window; (d) decline to use, share, or disclose any of that data for any purpose other than the deletion itself.
- **We do not knowingly collect personal information from children under 13.** We do not market to children. We do not include features designed to attract children.
- **If you are a parent or guardian** and believe your child has created an account on Parley despite our age requirements, email hello@parley.byexec.com with the username (or any identifying information) and we will respond within 7 days. We will provide reasonable parental access, deletion, and verification mechanisms as COPPA requires.

## 12. Security

We use industry-standard security practices: TLS for all transit, bcrypt for password hashing, WebAuthn for passkey support, rate limiting against brute force, JWT cookies marked HttpOnly + Secure + SameSite=Lax, force-logout when a user account is compromised. Our auth model + session handling were security-audited (red-team) in April 2026.

If you find a security vulnerability, please email hello@parley.byexec.com. We don't pay bug bounties at v1 but we credit responsible disclosures.

### Breach notification

If we discover a security incident that has compromised your personal data, we will notify you **without undue delay, and within 72 hours** where required by GDPR Article 33 or by US state law. Our notification will tell you, to the best of our then-current understanding:

- what categories of data were affected
- what we believe happened and when
- what we have done in response
- what we recommend you do (e.g., reset password, rotate API keys)
- how to contact us for follow-up

We may delay individual notification only when a law-enforcement agency advises us in writing that immediate notification would interfere with an investigation, and only for as long as that advisory remains in effect.

## 13. Government and law-enforcement requests

We require **valid legal process** before disclosing your account information or content to a government, court, or law-enforcement agency:

- **Subscriber data** (account creation date, email, last login IP) — produced on a valid subpoena.
- **Stored content** (messages, attachments, metadata about communications) — produced on a probable-cause search warrant issued by a court of competent jurisdiction. We do not produce stored content on a subpoena alone.
- **Real-time intercept** (live wiretap of communications) — only on a Title III intercept order, and only if technically possible given our architecture.
- **Emergency disclosure requests** (18 U.S.C. § 2702(b)(8) / § 2702(c)(4)) — we may voluntarily disclose limited information when we have a good-faith belief that an emergency involving imminent danger of death or serious physical injury requires it. Our standard requires the requesting agency to explain the emergency in writing — by email to hello@parley.byexec.com or via an established law-enforcement-portal submission. Oral or phone-only requests do not satisfy the writing requirement.

**We notify you** of legal demands targeting your account *unless* a court order, statute, or agency directive specifically prohibits us from doing so (e.g., a sealed warrant accompanied by a non-disclosure order under 18 U.S.C. § 2705(b)). When the prohibition lifts, we'll notify you retroactively where feasible.

**We do not voluntarily share account information** with law-enforcement agencies in the absence of valid legal process or a qualifying emergency.

**Transparency report.** If we receive law-enforcement demands of any kind, we'll publish an aggregate transparency report at parley.byexec.com/transparency at least annually, starting from the first calendar year in which we receive any such demand. We aim to publish each report within six months of the close of its reporting year. The report will include the number of requests received, the legal basis claimed, the number we complied with, the number we declined to comply with on legal grounds, and (where lawful) the number that targeted accounts of which the user was notified.

We don't have a dedicated legal team and we are a single-operator service. As a practical matter, our ability to formally challenge a federal demand is limited by resource constraints; "declined to comply" generally means we returned the demand to the requesting agency with a defect or scope objection rather than litigating it. We disclose this so the report's "challenged" number is read in context.

## 14. California Privacy Rights (CCPA / CPRA)

If you are a California resident, you have specific rights under the California Consumer Privacy Act (CCPA) as amended by the California Privacy Rights Act (CPRA). Most of those rights are already covered earlier in this Policy and in your account settings; this section pulls the CCPA-specific disclosures into one place for reference.

**Categories of personal information we collect.** In the past 12 months we have collected the following categories of personal information from users (mapped to the CCPA's categories):

| CCPA category | What we actually collect | Source |
|---|---|---|
| Identifiers | username, email, IP, user-agent, internal user ID | directly from you |
| Customer records | password hash, optional phone number, optional avatar/banner/bio | directly from you |
| Commercial information | none | — |
| Internet or other electronic activity | session metadata, message activity within Parley | directly from your use of Parley |
| Geolocation data | none beyond approximate region inferred from IP for security/abuse purposes | network |
| Audio / electronic / visual information | voice/video media (transient — see §5) | directly from you |
| Sensory data | none | — |
| Professional or employment information | none | — |
| Inferences | none | — |
| Sensitive personal information | none knowingly; passkey credential metadata if you enroll one | directly from you |

**Why we collect each category** is described in §2 above. **Retention period for each category** is described in §3 above (CPRA requires retention disclosure per category; §3 lists retention by data type, which maps onto the categories above — for each row of the table, the applicable retention is whichever §3 entry covers that data type).

**Categories of third parties we share with** are listed in §4 (Subprocessors). We do not sell or share personal information with third parties for cross-context behavioral advertising purposes — there is nothing for you to opt out of under "Do Not Sell or Share My Personal Information" because there is no such selling or sharing.

**Your CCPA rights** include:
- The right to **know** what we collect and how we use it (this Policy, plus self-serve export at Settings → Account → Download my data).
- The right to **delete** your personal information (Settings → Account → Delete my account, or email hello@parley.byexec.com).
- The right to **correct** inaccurate personal information (edit your profile in Settings, or email us).
- The right to **limit the use of sensitive personal information** — we do not use sensitive personal information beyond the purposes listed in §2, so this right does not have an additional opt-out for you to exercise.
- The right to **non-discrimination** for exercising any of these rights — exercising them does not affect your access to Parley or the price you pay (which is zero).

To exercise any of these rights, email hello@parley.byexec.com or use the in-product Settings flow. We will verify the request originates from your account and respond within 45 days.

## 15. Successor and acquisition

If Parley is acquired, merges with another entity, or undergoes bankruptcy or other restructuring, your personal data may be transferred to the successor or acquirer as part of the transaction. **We commit, while we are operating Parley, that we will not voluntarily transfer your personal data to any successor or acquirer that has not agreed to be bound by this Privacy Policy or one no less protective of your rights.** We will notify you in-app at least **30 days** before any such transfer takes effect, where feasible — and where 30 days isn't feasible (e.g., a court-supervised bankruptcy on a tighter timeline), we'll notify you as far in advance as the proceeding allows.

If the successor materially changes how your data is handled, the change will go through the standard substantive-change-to-this-Policy notice flow described in §16 below.

**Bankruptcy edge case.** Some past US bankruptcies have seen trustees attempt to sell user databases as assets over privacy-policy commitments. Courts have split on whether such commitments survive a Chapter 7 sale. We can't override how a bankruptcy court ultimately resolves that question, but we commit that, to the extent we have standing, we will object to any acquisition or court-supervised transfer that would weaken the protections in this Policy.

## 16. Changes to this Policy

When we update this Policy, we'll post the new version at parley.byexec.com/privacy and update the "Last updated" date at the top. For changes that affect what we collect or who we share with, we'll notify you in-app at least **30 days** before they take effect.

For non-substantive changes (typo fixes, clarifications that don't change meaning, contact-info updates), we'll just update the document and the "Last updated" date.

## 17. Contact

For all topics — general questions, privacy requests, data access / deletion / correction, security disclosures, accessibility issues, government-process service — email **hello@parley.byexec.com**. Routing within is informal at this stage; everything reaches a human.

You can also exercise your access and deletion rights self-serve from Settings → Account.

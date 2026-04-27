# Parley — Terms of Service

**Last updated: 2026-04-27**

These Terms govern your use of Parley, the chat platform operated at parley.byexec.com. By creating an account or otherwise using Parley, you agree to them.

We've tried to write these in plain language. Where lawyer-speak is unavoidable (the disclaimer, liability, and disputes sections), we've kept it as short as the law allows.

---

## 1. What Parley is

Parley is a chat platform for technical project communities — open-source maintainers, software teams, hobbyist makers, and the kinds of communities that gather around a shared technical interest. We provide the infrastructure: servers with text, voice, and code channels; direct messages; group DMs; bots; themes. You bring the project and the people around it. The platform-level rules in section 5 reflect that focus; they're not exhaustive, but they're what we think a community of that kind should expect of its host.

## 2. Who can use Parley

You must be **at least 18 years old**. We don't accept registrations from anyone younger. If we learn that an account belongs to someone under 18, we close it.

Parley is available worldwide except in **North Korea**, where US sanctions prohibit us from providing service. Everywhere else: you are responsible for complying with your local laws when you use Parley. We may add jurisdictions to the restricted list if a change in local law makes compliance infeasible; we'll notify affected users where we can.

## 3. Your account

You register with a username, an email address, and a password (or a passkey). You're responsible for keeping your credentials secure. Tell us at hello@parley.byexec.com if you suspect your account was compromised; we'll force-logout your sessions and help you reset.

You can have one personal account. You can also create **bot accounts** (separate logins, owned by your personal account, with their own API key) — see the developer docs for details.

You can leave at any time. Settings → Account → Delete my account erases your profile, friends, blocks, notifications, sessions, passkeys, and anything you uploaded. Your messages remain visible in the channels and DMs they were posted in but are reattributed to "Deleted User" so the conversation context survives. You can download a JSON archive of your data first via Settings → Account → Download my data.

## 4. Your content

**You own what you post.** You don't transfer ownership to us by using Parley.

You grant us a non-exclusive, royalty-free, worldwide license to host, transmit, display, and back up your content — only to the extent we need to operate the service for you and the people you sent it to. **Parley, as the platform operator, does not** sell your content, license it to third parties for advertising, or use it to train AI models. Bots configured by their owners may route your messages to third-party AI providers; their privacy practices apply to those transmissions, and the Privacy Policy lists each provider's training stance. See section 7 below and the Privacy Policy for the full picture.

If you delete a message, we delete it from our database. If you delete your account, see section 3.

## 5. Acceptable use

There are two kinds of rules: **server-level** (which the server owner sets) and **platform-level** (which we enforce regardless of which server you're in).

Platform-level rules are deliberately short. Most exist because the harm is *individual* — it targets a specific victim regardless of which server it happens in — and no server owner can police that on our behalf. The rest exist because Parley is a chat platform for technical project communities, and a few categories of content simply aren't part of that.

### Things we forbid platform-wide

- **Sexual or pornographic content.** Parley is a chat platform for technical project communities; sexual content isn't part of that. This is about platform focus, not a moral judgment — there are platforms built for adult content, and we're not one of them. Image uploads are scanned automatically; see Privacy Policy §6 for how that works.
- **Sexual content involving minors** (CSAM). Always reported to the National Center for Missing & Exploited Children (NCMEC) as US federal law requires.
- **Non-consensual intimate imagery** (NCII / "revenge porn"). Removed on report; account terminated.
- **Doxxing** — publishing someone's private personal information (home address, government ID, phone number, employer) without their consent for the purpose of harm.
- **Targeted harassment** — sustained, targeted abuse of a specific person, including coordinated harassment campaigns directed at an individual.
- **Real-world violence** — credible threats of violence, planning of violence, or content that incites imminent violence against identifiable people.
- **Malware, fraud, or platform abuse** — distributing malware, phishing, financial scams, attempts to compromise Parley's infrastructure (DDoS, exploit attempts, credential stuffing, etc.).
- **Impersonation** of a specific real person or organization in a way intended to deceive. Parody is fine when clearly labeled as parody.
- **Use by sanctioned individuals** — if you appear on the US OFAC SDN list, we cannot lawfully provide you service.

### What still fits Parley

These aren't moderated at the platform level — server owners may set their own rules in their own servers, and many do. They fit Parley because they're consistent with running a technical-community platform; they aren't "we're proud to be permissive" carve-outs.

- **Strong language, edgy humor, and dark themes.** Server-level rules apply within a server; we don't second-guess server owners' taste.
- **Political and religious speech.** We don't preemptively police viewpoint; specific server owners may set tone rules for their own servers.
- **Self-promotion and commercial use of the chat platform itself.** Run a server for your project, sell your software, link your portfolio, run a paid community via a third-party platform. Server owners may restrict it within their servers.
- **Discussion of legal-but-controversial topics** — drugs, firearms, weapons, gambling, sex work, cryptocurrency markets, etc. *Talking* about these is fine. *Using Parley as the primary venue for commerce in those categories* is off-topic — Parley exists for technical project communities, not as a marketplace for these items, and a server whose primary purpose is running such commerce on Parley will be treated as misusing the platform. The underlying activity must also remain legal in the jurisdictions of the people involved.
- **User automation, selfbots, and modified clients.** Use the API to automate your own account, build a custom client, run a bot from your account. Don't use automation to break the rules above (e.g., spam, harassment).

### Server-level moderation

Inside a server, the owner and the moderators they appoint set the rules. They can kick, ban, restrict channels, and remove messages within their server. If you're removed from a server, you can still use the rest of Parley.

### Reporting

If something violates the platform-level rules above: right-click the message → Report (in-app), or email hello@parley.byexec.com. We act on credible reports; we don't act on disagreement-with-content reports.

## 6. Server owners

If you create a server, you're its host. You decide who joins, who has what role, what gets posted. You're also the person we contact when someone in your server is breaking platform-level rules and the platform-level enforcement should land on the server too (e.g. the server is *primarily* dedicated to one of the forbidden categories).

We don't pre-screen server content. We respond to credible reports.

### What server owners must not do with member content

Server owners can see and — via Parley's API — extract the messages and metadata of members of their server. By running a server, you agree you will:

- **Not republish, sell, or otherwise commercially exploit member content** (messages, profile data, member lists) without the explicit consent of the members involved. Selling an established server's member list with the intent that the buyer contact those members is also prohibited.
- **Not transfer the server for monetary consideration** if the transfer would convey the member list, member roles, or channel content to a recipient who didn't already have access. The test is whether money or other value changes hands; transferring ownership to a co-moderator at no cost is fine.
- **Maintain reasonable security** for any API tokens or owner-level credentials tied to the server.
- **Disclose** to your members when you (or a bot you operate) are storing, exporting, or otherwise processing their messages outside Parley.

If you breach these expectations, we'll treat it as a violation of section 5 and suspend the server.

## 7. Bots, AI integrations, and bot-operator obligations

Some servers use bots. A bot is a separate account, owned by a Parley user, with its own API key. Bots can be plain (responding to commands), AI-driven (forwarding messages to an LLM), or both.

### When you talk to a bot

When you message a bot, your message is delivered to the bot endpoint configured by its owner (typically code running on the owner's infrastructure — not the owner reading it personally). If the bot is AI-driven, that code then forwards your message to the AI provider the bot's owner configured — Parley's default (Ollama Cloud) or, if the owner brought their own API key, Anthropic, OpenAI, xAI, Mistral, or Google. Each provider's privacy policy applies to that transmission. See our Privacy Policy "AI processing" section, including the free-tier-vs-paid-tier distinction for Google Gemini and Mistral, which is a real footgun worth understanding.

The default Parley AI provider does not use your messages to train models. Third-party providers vary; we list each one's stance.

### Bot-operator obligations

If you run a bot — a bot account you created and configured, or a third-party bot you installed in your own server — these additional terms apply to you in addition to the user-facing terms above:

- **You are *a* data controller** for messages users send to your bot, in addition to Parley's own role as a controller for transmission and abuse-handling. (We don't disclaim our own controller obligations by virtue of your having one — we act as independent controllers, each responsible for our own processing purposes.) Treat user messages with at least the care our Privacy Policy describes for Parley's handling of user content.
- **Don't store user messages outside Parley** unless you have a legitimate reason (e.g., your bot needs to remember context across sessions) and you've disclosed it. Even with disclosure, retain only the minimum needed and for the minimum time.
- **Don't train models on user messages** without explicit, informed, opt-in consent from each user whose message would be used. Default-on is not consent.
- **Don't sell or transfer user message content** to third parties for any purpose.
- **Disclose what your bot does with messages.** If your bot calls an external service, calls a model that retains data, or stores anything beyond the message that triggered it, the bot's profile or channel topic should say so in plain language.
- **List a privacy contact.** Your bot's profile must include a contact (email or otherwise) reachable by users whose messages your bot processes, so they can exercise the rights below.
- **Honor deletion.** When a user deletes their Parley account, asks you to delete their data, or otherwise withdraws consent, do so promptly.
- **Use the API key as documented.** Keep it secret; don't share it; rotate it if it's compromised; don't let anyone else use it.

If we receive credible reports that a bot operator is violating these obligations, we'll suspend the bot and may suspend the operator's user account.

### What bot operators must not do

The platform-level forbidden list in section 5 applies to bots. A bot that distributes malware, harasses specific people, or scrapes profile data for resale is a section-5 violation regardless of the human's intent.

## 8. Account suspension and termination

We can suspend or terminate your account if you break the platform-level rules in section 5. We try to be proportionate — a warning for first-time low-severity issues, account termination for serious or repeated ones. If you think we got it wrong, email hello@parley.byexec.com and we'll review.

When we terminate an account, your content is handled the same way as a self-serve deletion (section 3): profile and personal data deleted; messages anonymized so others' conversations survive.

We can also suspend service to comply with a valid legal order, sanctions developments, or to address a security incident.

## 9. The boring legal parts

### 9.1 As-is, no warranty

Parley is provided **"as-is"** and **"as available"** without warranties of any kind, either express or implied, including without limitation any implied warranties of merchantability, fitness for a particular purpose, non-infringement, or availability. We don't promise the service will be uninterrupted, error-free, secure against all attacks, or that any specific feature will keep working. We'll do our best. The service can go down, lose messages in flight, change, or be retired.

### 9.2 Limitation of liability

To the maximum extent allowed by applicable law, **Parley's total cumulative liability to you for any and all claims arising out of or related to these Terms or your use of the service is limited to one hundred US dollars (USD $100)**. To the extent that limit is not enforceable in your jurisdiction, our liability is the minimum allowed by the law of that jurisdiction.

We are not liable for indirect, incidental, special, consequential, exemplary, or punitive damages, including without limitation loss of profits, loss of data, loss of goodwill, business interruption, or substitute service costs, even if we have been advised of the possibility of such damages.

This limitation applies regardless of the legal theory the claim is brought under (contract, tort, statute, or otherwise) and survives termination of these Terms.

### 9.3 Indemnification

If your use of Parley causes a third party to bring a claim, suit, or proceeding against us — for example, you posted content that infringed someone's copyright, or you used the service in a way that violated applicable law — you agree to indemnify us, defend us at your expense, and hold us harmless from the resulting damages, settlements, and reasonable attorneys' fees.

### 9.4 Disputes — informal resolution first, then binding arbitration

**This section affects your legal rights. Read it carefully.**

**Informal resolution (required first step).** Before either of us starts a formal proceeding, we agree to try to resolve the dispute by writing to hello@parley.byexec.com (for you to reach us) or to the email address on your account (for us to reach you). We'll work in good faith for at least **30 days** from the date of that first email before either of us escalates.

**Small-claims court.** Either of us may bring a qualifying claim in a small-claims court of competent jurisdiction. Nothing in this section requires you to arbitrate a small-claims-eligible dispute.

**Binding individual arbitration.** Any other dispute, claim, or controversy between you and Parley arising out of or relating to these Terms or your use of the service — including questions about the validity, scope, or enforceability of this arbitration agreement itself — must be resolved by **binding individual arbitration administered by the American Arbitration Association ("AAA") under its Consumer Arbitration Rules**, available at adr.org. The arbitration will be conducted by a single arbitrator. The arbitrator's decision is final and binding on both of us, subject only to the limited judicial review the Federal Arbitration Act allows.

**You and Parley each waive the right to a jury trial.**

**Class-action waiver.** **You and Parley each waive the right to participate in any class action, collective action, mass action, consolidated action, or representative action of any kind.** All disputes must be brought in your individual capacity and not as a plaintiff or class member in any purported class, collective, or representative proceeding. The arbitrator may not consolidate more than one person's claims and may not preside over any form of representative proceeding.

**Mass-arbitration coordination.** If 25 or more individual arbitration demands raising materially similar claims are filed against Parley by counsel acting in coordination, the parties agree the AAA Mass Arbitration Supplementary Rules apply.

**Limitation period.** Any claim arising out of or related to these Terms must be filed within **one (1) year** after the claim arose. Claims filed after that period are permanently barred, to the maximum extent allowed by law.

**Confidentiality.** The existence and content of any arbitration proceeding (including pleadings, evidence, and the award) are confidential between the parties, except as needed for enforcement, judicial review, or as otherwise required by law.

**Opt-out window.** You may opt out of this arbitration agreement and the class-action waiver within **30 days of first creating your Parley account** (or, for accounts created before this version of the Terms took effect, within 30 days of these Terms' effective date). To opt out, email hello@parley.byexec.com from the address on your account with your username and the words "arbitration opt-out." Opting out does not affect any other part of these Terms, and does not affect future dispute-resolution arrangements you choose to enter into.

**EU / UK consumers.** If you are a consumer resident in the European Union, the European Economic Area, or the United Kingdom, this section applies only to the extent permitted by your local consumer-protection law. You retain the protections of EU/UK consumer law, including any non-waivable rights to litigate in your place of residence. Nothing here purports to override that.

**Severability.** If any portion of this Disputes section is held unenforceable in a particular case, the rest of the section stands. If a court of competent jurisdiction finds the class-action waiver unenforceable as to a particular claim, the entire Disputes section is null *as to that claim only*, and that claim proceeds in court — but the substantive limits in the rest of these Terms (sections 9.1, 9.2, 9.3) still apply.

### 9.5 Governing law

These Terms and any dispute arising out of or related to them are governed by the substantive laws of the **State of Illinois**, without regard to its conflict-of-law principles. The Federal Arbitration Act (9 U.S.C. § 1 et seq.) governs the interpretation and enforcement of section 9.4.

For any non-arbitration claim that ends up in court (e.g. small-claims), venue is exclusive to the state and federal courts located in **Cook County, Illinois**, and you and we each consent to personal jurisdiction there.

## 10. Copyright (DMCA)

Parley respects intellectual property rights and complies with the U.S. Digital Millennium Copyright Act ("DMCA"), 17 U.S.C. § 512.

If you believe content on Parley infringes your copyright, send a DMCA notice meeting the requirements of 17 U.S.C. § 512(c)(3) to our Designated Agent:

> **DMCA Designated Agent**
> Dylan Hart
> [physical address — to be provided to U.S. Copyright Office at registration; published here once filed]
> Email: hello@parley.byexec.com

A valid notice must include all six elements 17 U.S.C. § 512(c)(3) requires: a physical or electronic signature; identification of the copyrighted work; identification of the allegedly infringing material with sufficient detail to locate it (a Parley message URL is ideal); your contact information; a statement that you have a good-faith belief the use is not authorized; and a statement, under penalty of perjury, that the information is accurate and that you are authorized to act on the copyright owner's behalf.

We respond to valid notices by removing or disabling access to the allegedly infringing content. We provide the affected user with a copy of the notice and an opportunity to file a counter-notice. Repeat infringers' accounts are terminated.

If you believe your content was removed in error, you may file a counter-notice meeting the requirements of 17 U.S.C. § 512(g)(3) at the same address. We will restore the content within 10–14 business days unless we receive notice that the original complainant has filed an action in court.

We have registered our DMCA Designated Agent with the U.S. Copyright Office's DMCA Designated Agent Directory (a registration link will be added here once filed).

## 11. Changes to these Terms

We can update these Terms. When we do, we'll post the updated version at parley.byexec.com/terms and update the "Last updated" date at the top.

For substantive changes — anything that materially affects your rights or obligations — we'll notify you in-app **at least 30 days** before the change takes effect. Continued use after the effective date means you accept the changes. If you disagree, you can delete your account before the change takes effect.

For non-substantive changes (typo fixes, clarifications that don't change meaning, contact-info updates), we'll just update the document and the "Last updated" date.

## 12. Accessibility

We try to make Parley usable for everyone. The desktop and web clients aim to support keyboard-only navigation, screen-reader semantics on interactive controls, and respect operating-system accessibility settings (reduced motion, increased contrast, large text). Accessibility is not a finished thing — there are gaps, and we treat reports as bugs.

If you encounter an accessibility barrier or want to report something we got wrong, email hello@parley.byexec.com with "accessibility" in the subject. We aim to respond within 7 days and to fix what we can.

## 13. Miscellaneous

- **Entire agreement.** These Terms (together with the Privacy Policy) are the entire agreement between you and Parley regarding the service and supersede any prior agreement.
- **No waiver.** Our failure to enforce any provision is not a waiver of that provision.
- **Assignment.** You may not assign these Terms without our consent. We may assign them to an acquirer of our business or to a successor entity. If we do, we'll notify you in-app at least 30 days in advance where feasible (see also Privacy Policy §15 for the corresponding data-handling commitment).
- **Severability.** If any provision is held invalid or unenforceable, the rest stands.
- **Notices to us.** All formal notices to Parley must be sent to hello@parley.byexec.com.
- **Notices to you.** All formal notices from Parley to you may be sent to the email address on your account.

## 14. Contact

For all topics — general questions, security disclosures, abuse reports, DMCA notices, privacy requests, accessibility reports, support — email **hello@parley.byexec.com**. Routing within is informal at this stage; everything reaches a human.

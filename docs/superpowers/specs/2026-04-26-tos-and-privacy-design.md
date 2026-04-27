# Terms of Service + Privacy Policy Design

**Goal:** Ship publishable Terms of Service and Privacy Policy as v1-launch hard-blockers. Cover GDPR, DSA, CCPA, COPPA non-applicability, and DMCA safe-harbor prerequisites.

**Artifacts produced (this is the deliverable):**
- `/TOS.md` — final-draft Terms of Service
- `/PRIVACY.md` — final-draft Privacy Policy
- `/README.md` — pre-launch checklist banner added at top
- This spec — reasoning record

**Out of scope for this spec:** the implementation plan (rendering at `/terms` and `/privacy`, footer links, DMCA filing, free-tier warning UI, NK geoblock). Those are tracked separately and live in their own implementation skill pass.

---

## Decisions made

### Platform philosophy
**"Mature defaults"** (option B from the brainstorming menu). Hard floor on illegal-everywhere harms (CSAM, NCII, doxxing, real-world violence threats, malware, sanctioned-individual evasion); explicit allowance of NSFW (channel-labeled), political speech, commercial use, selfbots/automation, controversial-but-legal topics, crypto. Server-level moderation handles in-server norms; platform handles individual-victim harms.

### Age policy
**18+ only.** Date of birth captured client-side at signup, age computed locally, only `is_adult: true` boolean transmitted to server — server stores no DOB. Avoids COPPA, GDPR-K, KOSA, and UK AADC obligations entirely. Operationally simpler with NSFW-allowed posture (no per-channel age verification needed since everyone on platform is age-attested).

### Geographic restrictions
Open everywhere except **North Korea** (only country with comprehensive US sanctions and no personal-communications general license). Personal-communications General Licenses cover Russia, Iran, Cuba, Syria, occupied Ukraine territories — all explicitly authorized for free social-platform use. TOS includes "you are responsible for complying with your local laws" disclaimer.

### Data + privacy
- No training on user content (verified: Ollama Cloud + every BYOK provider's policy).
- No analytics or telemetry in v1; if added later, self-hosted privacy-conscious aggregate-only.
- Subprocessors documented with verbatim quotes from each provider's public policy: Brevo, Cloudflare, LiveKit, Ollama Cloud (primary), Anthropic, OpenAI, xAI, Mistral, Google (BYOK secondary).
- Voice/video not recorded.
- One essential session cookie; no third-party cookies, no banner needed under EU law.
- Mistral and Google Gemini have free tiers that train on inputs/outputs by default; UI warning at bot-config-time discloses this (task #64).

### Liability posture
**Hard.** User explicitly chose maximum protection because the service has no financial relationship with users.
- Mandatory binding individual arbitration via AAA Consumer Rules.
- Class-action waiver. Jury-trial waiver.
- Mass-arbitration coordination via AAA Mass Arbitration Supplementary Rules at 25-claim threshold.
- 1-year limitation period on claims.
- Confidentiality of arbitration proceedings.
- Small-claims carve-out preserved (required for clause enforceability under existing case law).
- 60-day informal-resolution requirement before formal escalation.
- $100 nominal liability cap; "no liability beyond what the law requires" framing.
- Severability if any portion held unenforceable.

### Governing law
**Illinois.** All Parley infrastructure is in Illinois. Federal Arbitration Act governs arbitration; Illinois substantive law governs everything else. Venue exclusive to Cook County, Illinois state and federal courts.

### Operational mechanics
- 30-day notice for substantive ToS changes.
- 30-day server log retention.
- 30-day database backup retention.
- Single `hello@parley.byexec.com` contact at v1; routes everything to one human inbox.
- Suspension policy: warning for first-time low-severity, termination for serious or repeated. Email-based appeal route.

### DMCA
Designated Agent section drafted into TOS §10 with all required notice elements. **Pre-launch action item:** register the agent with the U.S. Copyright Office (online form, $6 fee, 3-year renewal) at <https://www.copyright.gov/dmca-directory/>. README banner tracks this.

---

## Implementation followups (not part of this spec)

These are tracked elsewhere; listed here only so the reasoning chain captures what shipping the policy *enables*:

1. Render TOS + Privacy at `/terms` and `/privacy` on the public site (currently only available as repo files).
2. Footer + settings links to `/terms` and `/privacy`.
3. Signup-time consent: existing form already requires acceptance via terms — verify it points at `/terms`.
4. Free-tier warning UI at bot configuration time (task #64).
5. Cloudflare WAF rule blocking signups from North Korea.
6. User-facing report flow (separate v1 launch dependency, also referenced in TOS §5).
7. DMCA Designated Agent registration with U.S. Copyright Office (pre-launch checklist in README).

---

## Sourcing and verification notes

- Subprocessor policy quotes captured 2026-04-26 from publicly published policy pages. Brevo's policy is JS-rendered and not server-readable; that section is honestly disclosed as "could not capture verbatim quotes" with a path forward (manual transcription from a real browser).
- During the policy-fetch pass, the Anthropic privacy policy page returned a fabricated `<system-reminder>` block attempting to alter the date — a prompt-injection attempt against an LLM-driven scraper. The research agent caught and ignored it. Worth being aware of for future Anthropic-page fetches.
- Legal-pass disclaimer: the user is the sole platform operator; this is not legal advice. A licensed attorney should review before scaling past ~1000 MAU. At v1 (small launch, single operator), iterative drafting is the normal path.

---

## v2 changes (2026-04-26 — second pass after external review)

A clean instance of the assistant reviewed the v1 drafts and flagged real gaps. The following changes were made in response. *(Full reviewer note retained at `~/NOTE_TO_REVIEWER.md` for the next external review pass.)*

**Substantive additions:**

- **Bot-operator obligations** (TOS §7). The v1 draft treated bot owners as data passers. Reality: they're data controllers for the messages users send to their bot. New section spells out that they may not store/redistribute user content beyond what's needed, may not train models on it without explicit opt-in, must disclose what their bot does with messages, must honor deletion requests.
- **Server-owner data-handling expectations** (TOS §6, new subsection). Server owners can extract member content; new clause prohibits republishing/selling member content, prohibits trafficking-of-servers (sale of established servers with member lists), requires reasonable security on owner-level credentials, requires disclosure when storing/exporting messages outside Parley.
- **Government and law-enforcement requests** (PRIVACY §13, new). Spells out the standard ECPA process — subpoena for subscriber data, warrant for stored content, Title III for real-time intercept, 18 U.S.C. § 2702(b)(8) emergency-disclosure framework. Commits to user notification unless gagged. Commits to an annual aggregate transparency report once any demand is received.
- **Breach notification** (PRIVACY §12, expanded). 72-hour commitment where GDPR or US state law requires; describes what we'll tell users; carve-out for law-enforcement directives that delay individual notification.
- **Successor / acquisition** (PRIVACY §15, new). Data may transfer to successor in acquisition/bankruptcy; successor bound by this Policy or one no less protective; 30-day advance notice where feasible. Cross-referenced from TOS §13 Misc.
- **California Privacy Rights / CCPA addendum** (PRIVACY §14, new). Aggregates the disclosures CCPA/CPRA wants in one place: categories of personal information collected (mapped to the statutory list), categories of sources, categories of third parties shared with, statutory rights, the "no sale or share for behavioral advertising" position with explicit "nothing to opt out of" framing.
- **COPPA expansion** (PRIVACY §11). Specific under-13 handling: account closure, data deletion, backup propagation, parental contact mechanism. Even though we don't accept under-18s, COPPA's "actual knowledge" trigger requires us to spell out what we do when we discover one.
- **Sensitive-log retention specifics** (PRIVACY §3). CSAM-related records: 90 days for the report, 1 year for retained images and metadata, per 18 U.S.C. § 2258A(h). Trust-and-safety case files: 1 year after closure. Audit logs: tied to entity lifetime. Session IPs/UAs: tied to session-cookie lifetime.
- **Accessibility statement** (TOS §12, new). Brief commitment to keyboard nav, screen-reader semantics, OS accessibility-setting respect; reports to hello@ with 7-day response commitment.

**Arbitration / disputes refinements** (TOS §9.4):

- **30-day opt-out window** added. Improves enforceability under existing case law (especially Ninth Circuit) and gives EU-leaning users a path to opt out without nullifying the clause for everyone.
- **EU/UK consumer carve-out** added. Clause applies "only to the extent permitted by your local consumer-protection law"; EU/UK consumers retain their non-waivable forums. This makes the clause more enforceable in those jurisdictions, not less.
- **30-day informal-resolution period** (was 60 days). 60 was too long given the $100 cap.

**Internal consistency repairs:**

- TOS §4 ("we don't... train AI models") now explicitly carves out third-party AI providers configured by bot owners and points at the Privacy Policy. Removes the apparent contradiction with PRIVACY §6.

**Smaller wording fixes:**

- "device fingerprint" → "user-agent string" (PRIVACY §1). The v1 wording was technically alarmist and inaccurate.
- TOS §2 now reserves the right to add jurisdictions to the restricted list if local-law compliance becomes infeasible.

**Items deliberately not changed despite reviewer suggesting them:**

- **"Real-world violence" as incitement-only, not glorification.** The reviewer suggested expanding to glorification. Kept as incitement-only to stay consistent with the platform's permissive-speech posture; expanding to glorification opens content-moderation judgment-calls that conflict with the stated ethos. Server owners can ban glorification within their own servers.

**Items deferred to pre-launch checklist (in `README.md`):**

- Building the age gate UI. PRIVACY §11's claim that "DOB is computed locally, only is_adult is transmitted" is forward-looking — confirmed via code inspection that no age gate exists today. README banner gates launch on building it.
- Manual transcription of Brevo policy quotes (their privacy page is JS-rendered SPA, not server-readable).

---

## v3 changes (2026-04-26 — third pass after second external review)

The clean reviewer instance reviewed the v2 packet and surfaced one substantive bug, several tightenings, and one operational risk we acknowledge but can't fix in the document.

**Substantive bug fix:**

- **18 U.S.C. § 2258A(h) citation corrected.** The REPORT Act (Pub. L. 118-59, signed May 7, 2024) struck "90 days" and inserted "1 year" in § 2258A(h)(1). v2's text — "90 days for the report itself and one year for retained images and metadata" — was wrong on both numbers and on the bifurcation (the statute doesn't bifurcate). v3 says: one year for the contents of a CyberTipline report, full stop, citing the REPORT Act amendment. Verified via Pub. L. 118-59 and Cornell LII.

**Tightenings made in v3:**

- **Bot-operator controllership framing softened.** v2 said "you are *the* data controller" — could be read as Parley disclaiming its own controller obligations. v3 says "you are *a* data controller... in addition to Parley's own role as a controller for transmission and abuse-handling. We're parallel controllers, each for our respective purposes." (TOS §7)
- **Bot-operator privacy-contact requirement added.** Without a working contact, the "honor deletion" obligation has no path. v3 requires bot's profile to list a contact for privacy requests. (TOS §7)
- **Server-trafficking test sharpened.** v2's "non-commercial succession reasons is fine" was slippery. v3: "the test is whether money or other value changes hands; transferring ownership to a co-moderator at no cost is fine." (TOS §6)
- **TOS §4 wording strengthened.** v2 said "Parley itself does not... train AI models" then immediately carved out third-party providers. v3 says "Parley, as the platform operator, does not... Bots configured by their owners may route your messages to third-party AI providers" — closes the "Parley enables training-by-others" argument.
- **EU/UK arbitration carve-out wording tightened.** v2 cited "the right to bring proceedings in the courts of your place of residence" — that's a Brussels I-bis Article 18 phrasing for *defendants*, not for the user as plaintiff. v3 says "the protections of EU/UK consumer law, including any non-waivable rights to litigate in your place of residence." (TOS §9.4)
- **Accessibility commitment softened.** v2 said "we commit to responding within 7 days." v3 says "we aim to respond within 7 days" — accurate for a single-operator service. (TOS §12)
- **Privacy §1 IP claim narrowed.** v2's "not used for analytics" was technically inconsistent with the §3 disclosure that HTTP request logs (which contain IPs) rotate after 30 days. v3 explicitly distinguishes session-IP (used for hijack detection) from request-log IP (used for security/abuse, rotated after 30 days).
- **Privacy §10 mentions EU-US Data Privacy Framework alongside SCCs.** v2 only cited SCCs. v3: DPF where the subprocessor is self-certified, SCCs otherwise — matches Cloudflare's quoted text.
- **Privacy §13 transparency-report timing.** v2's "at least annually starting from the first calendar year" was right in shape but unbounded on cadence. v3 adds "we aim to publish each report within six months of the close of its reporting year." Also: "challenged" softened to "declined to comply with on legal grounds," with a transparent acknowledgment that single-operator resource constraints limit the ability to formally challenge federal demands.
- **Privacy §13 emergency-disclosure writing requirement.** v2 just said "in writing." v3 specifies "by email to hello@parley.byexec.com or via an established law-enforcement-portal submission. Oral or phone-only requests do not satisfy the writing requirement."
- **Privacy §14 retention disclosure for CCPA.** v2's categories table didn't satisfy CPRA's per-category retention disclosure. v3 adds a pointer line directing readers to §3 for retention by data type, and explains the mapping.
- **Privacy §15 acquisition / bankruptcy aspirational language.** Added "to the extent we have standing, we will object to any acquisition or court-supervised transfer that would weaken the protections in this Policy" — acknowledges the RadioShack / Toysmart bankruptcy edge case and signals intent without overclaiming.

**README pre-launch checklist additions:**

- "Notice at Collection placement on signup" — CCPA wants the Privacy Policy link adjacent to the submit button on the registration form, not just in the footer.

**Operational risks acknowledged (not fixable in the document):**

- The "targeted harassment" rule will require the most case-by-case judgment of any rule in §5, given Parley's broader "no civility rule, no misinformation rule" stance. This is an operational reality, not a doc gap.
- A server *primarily* dedicated to glorifying (rather than inciting) violence is an edge case the platform-level rules don't cover; "we don't moderate glorification at the platform level" will land badly with reporters in a media-event scenario. Documented but not changed — operator's deliberate stance.
- Audit-log export is referenced via "Parley's API" rather than as a built feature, since no UI export of server-side audit logs exists today (verified via grep). User-side export ships as `GET /api/me/export`.
- Cloudflare's training stance on its own ML features (bot management, AI Gateway) is undefined in their public policy. v1.1 follow-up: confirm which Cloudflare features Parley actually uses and whether any of them sends customer data to a model.

---

## v4 changes (2026-04-26 — final polish after third external review)

The v3 reviewer returned with "ship it" + three small refinements. All three applied; no further round trip planned.

- **TOS §7 "When you talk to a bot" routing description.** v3 wording read as if the human bot-owner sees every DM personally. v4 clarifies it's machine processing: "your message is delivered to the bot endpoint configured by its owner (typically code running on the owner's infrastructure — not the owner reading it personally)."
- **TOS §7 controllership term of art.** v3 said "parallel controllers, each for our respective purposes." v4 swaps to "we act as independent controllers, each responsible for our own processing purposes" — "independent" is the standard GDPR term (vs. joint, which would create Art. 26(3) joint-and-several liability we don't want).
- **PRIVACY §15 bankruptcy commitment.** v3 said the successor "will be bound by this Privacy Policy or one no less protective" — read as a *prediction* about the successor's future obligations, not a *binding present-tense commitment* by Parley. v4 converts to: "We commit, while we are operating Parley, that we will not voluntarily transfer your personal data to any successor or acquirer that has not agreed to be bound by this Privacy Policy or one no less protective." Same outcome, sturdier framing — a court can hold Parley to a present-tense commitment.

After v4, three reviewer passes have closed: the v1 reviewer's 13+7 items, the v2 reviewer's substantive bug + 6 tightenings, and the v3 reviewer's 3 polish items. The DPF wording was confirmed durable as of early 2026 (General Court dismissed the Latombe challenge in September 2025; CJEU appeal pending; Commission published updated DPF FAQ January 15, 2026). The §2258A citation was confirmed accurate against Pub. L. 118-59.

The remaining launch-gating items live in the README pre-launch checklist (build the age gate, file DMCA Designated Agent, capture Brevo policy quotes manually, configure NK Cloudflare WAF, build report flow, build free-tier API-key warning UI, render docs at /terms and /privacy, place Notice at Collection adjacent to signup submit button).

---

## v5 changes (2026-04-27 — platform-identity pivot)

Operator decision: Parley's brand narrows from "more permissive than Discord with mature defaults" to **"a chat platform for technical project communities."** This isn't a small wording tweak — it changes the platform's posture on NSFW (from labeled-allowed to outright forbidden), the framing of the "still allowed" rules in §5 (no longer permissive-by-design carve-outs, but consistency with the technical-community focus), and the §6 image-processing disclosure (which now describes a 3-layer image-content moderation pipeline).

**TOS:**

- **§1** rewritten to anchor identity: *"Parley is a chat platform for technical project communities — open-source maintainers, software teams, hobbyist makers..."* Removes v4's "we try not to have opinions about what you use it for" framing, which reads aspirational under the new posture.
- **§5 "Things we forbid platform-wide"** adds:
  > **Sexual or pornographic content.** Parley is a chat platform for technical project communities; sexual content isn't part of that. This is about platform focus, not a moral judgment — there are platforms built for adult content, and we're not one of them. Image uploads are scanned automatically; see Privacy Policy §6 for how that works.
- **§5 "Things we explicitly allow"** renamed to **"What still fits Parley"** and reframed: not "we're more permissive than X" but "these are consistent with running a technical-community platform." The NSFW bullet is removed (now forbidden); the "no misinformation rule, no civility rule" boast is dropped (drew flack from each external reviewer and is now simply absent rather than narrated). The "Cryptocurrency, NFTs, and adjacent commerce" standalone bullet is folded into the legal-but-controversial bullet — there's no need for a defensive solo bullet about crypto under the new framing.
- **§5 legal-but-controversial topics** gains the **primary-purpose-commerce caveat**: *talking* about controversial-but-legal topics is fine; *using Parley as the primary venue for commerce* in those categories is off-topic, and a server whose primary purpose is such commerce will be treated as misusing the platform. This closes a loophole that mattered less under the old "permissive-but-common-sense" framing.

**PRIVACY:**

- **§4 Cloudflare** entry expanded with a paragraph on Cloudflare's **CSAM Scanning Tool** (hash-matches cached image content against NCMEC's NGO and industry hash lists; positive matches go to a notification address at Parley and trigger a CyberTipline report per §3 / 18 U.S.C. § 2258A). Documented as part of Cloudflare's existing service offering — no separate DPA required.
- **§4 Hive Moderation** added as a new subprocessor with a **stub disclosure** (same honest-disclosure pattern as the original Brevo stub before quotes were captured). Stub explicitly says: image-upload feature stays disabled at launch until Hive's DPA is captured here in plain text. README pre-launch checklist gates launch on this.
- **§6 renamed** from "AI processing" to "AI and image-content processing" and split into three subsections: **AI inference** (existing content unchanged), **Image-upload scanning** (the new 3-layer pipeline disclosure with explicit positive-match handling for adult-content vs. CSAM), and **What we don't do at the platform layer** (we don't run text-side classifiers, no sentiment analysis, no profile-building). The image-scanning subsection follows the reviewer's drafted wording closely with the addition of explicit retention/audit-log pointers to §3.

**README pre-launch checklist:**

- Image upload disabled by default until 4-layer pipeline verified end-to-end in production.
- Hive Moderation DPA captured into PRIVACY §4.
- Cloudflare CSAM Scanning Tool enabled and notification email round-tripped.
- NCMEC reporter registration completed.

**Operational implications acknowledged but not fixed in the doc:**

- The image-moderation pipeline is its own implementation effort and gets its own design spec at `docs/superpowers/specs/2026-04-26-image-upload-moderation-design.md` (per the reviewer's deliverable list). This v5 update covers only the policy-document side; the code lives in a separate plan.
- The "no civility rule, no misinformation rule" framing is gone from §5. We still don't have either rule — but we no longer narrate that absence, which had read as the platform's tone-setter and was inconsistent with the technical-community pivot.
- Existing servers may today contain content that is now forbidden under the new §5 (e.g., NSFW labeled channels). Enforcement at v1 launch will be report-driven for existing content; the automatic image-moderation pipeline is forward-looking once image upload is re-enabled. The README pre-launch checklist explicitly gates image upload on the pipeline being verified.
- The platform-identity anchor in §1 is a public statement; if the operator later wants to broaden Parley's audience (e.g., to general-interest community hosting), the §1 anchor and the §5 "primary-purpose commerce" caveat would both need to be revisited together, since they're load-bearing on the new posture.

---

## What this design does NOT cover

- Per-region adapted legal text (e.g. translated French/German versions). v1 ships English only; users in other jurisdictions read the English version under the "your local laws are your responsibility" disclaimer.
- Bug bounty program. Mentioned in privacy as "no bounties at v1, but credit responsible disclosures." Not a separate program document.
- Acceptable-Use bot/API policy detail. Section 7 of TOS covers AI bots; the developer docs at `docs/bots.md` cover the API-level acceptable-use specifics.
- Cookie banner. Deliberately omitted because Parley uses only essential session cookies (exempt under EU law).
- Children's-safety appeals workflow. Out of scope for an 18+ platform.

// Package gitprovider is the Git Provider SPI (version "gitprovider.v1"): the
// contract a Paca plugin implements to adapt one source-control platform
// (GitHub, GitLab, Bitbucket, Gitea, …) to Paca's platform-neutral Git
// integration.
//
// Paca core owns everything that is not platform-specific — the webhook
// endpoint and inbox, trigger routing, actor policy, agent runs, status
// comments, audit, the MCP tools and the UI. A provider only answers the
// platform-specific questions: how to verify and parse a webhook, how to mint a
// credential, how to read and write change requests, reviews, comments and
// checks.
//
// # Writing a provider
//
//  1. Implement [Provider] (required operations) and any of the optional
//     interfaces ([ThreadResolver], [FileReader], …) the platform supports.
//  2. Return an honest [Info]: every optional interface you implement must have
//     its capability flag set, and every flag you set must be backed by an
//     implementation. The conformance suite checks both directions.
//  3. Mount it from your plugin's Init with serve.Register(ctx, provider)
//     (package gitprovider/serve).
//  4. Pass the conformance suite (package gitprovider/conformance) using
//     recorded webhook fixtures and a fake HTTP backend. The reference
//     implementation in gitprovider/fakeprovider shows the expected behaviour.
//
// # Conventions
//
//   - Operations that do not apply return [ErrUnsupported] (and the matching
//     capability is false); core never calls them.
//   - Map platform failures onto the [ErrorCode] set so core can retry and back
//     off uniformly. Never include credentials or webhook secrets in an error.
//   - Writes accept an IdempotencyKey; honour it where the platform allows and
//     say so with [Capabilities.IdempotentWrites].
//   - Text that originates from users or repository content (comment bodies,
//     diffs, logs, file contents) is untrusted. Return it verbatim; core fences
//     it before it reaches a model.
package gitprovider

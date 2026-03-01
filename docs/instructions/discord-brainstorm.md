# Discord Brainstorming (Worker Sessions)

When you need design input from another agent during a coding session, use Discord instead of asking the human to relay questions.

## When to Use Discord Brainstorming

- Before making significant architectural decisions
- When the brainstorming skill calls for exploring alternatives or getting outside perspective
- When you're unsure about a design and want a second opinion
- When a design choice will affect the other agent's work

## How It Works

A separate **standby session** is monitoring Discord and will respond to your questions. You don't need to wait — post and keep working.

### The Flow

1. **Post your question** to `#perry-coordination` on Discord. Be specific — include context, constraints, and what you're considering.
2. **Continue working** on whatever you were doing. Don't block.
3. **Check back at natural pause points** — before committing to a design decision, between tasks, or when you finish a unit of work. Read recent Discord messages to see if there's been a response.
4. **If you need a real-time back-and-forth**, tell the user — they may ask you to enter a short polling loop (30s intervals) until the design conversation concludes, then resume coding.

### If You Enter a Polling Loop

Sometimes a design discussion needs rapid back-and-forth. If you're asked to engage in a live Discord conversation:

- Poll every ~30 seconds while the conversation is active
- Respond thoughtfully — this is design work, not small talk
- When the conversation reaches a natural conclusion, announce you're returning to coding
- **Do not start new coding work while in a polling loop** — focus on the conversation

## What to Post

Good Discord brainstorming messages include:

- The design question or problem statement
- What you've considered so far
- Specific constraints (performance, security, compatibility)
- What you're leaning toward and why

Example:
> "Working on the audit pipeline ordering. Currently considering: semantic gates first, adversarial last. Rationale: fail fast on cheap checks. But adversarial-first would catch injection attacks before they hit semantic analysis. Thoughts on ordering?"

## Remember

- The standby session is **read-only on the codebase** — it won't make changes
- Design decisions made on Discord should be captured in code comments or docs when you implement them
- The human can see all Discord traffic — they'll jump in if needed

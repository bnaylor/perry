# Additional requirements
## Command and control, status
- We'll build on the Discord skills we've already got to have the primary human <> platform communication occur there
    - Exact personas to expose on discord from the system TBD - let's discuss this
- Discord should not be sent verbose logs or other large output, but status messages can reference a log-viewing
interface or some other diagnostic path
- One of the major processes in the system is an always-on discord bot that proxies status and commands 
- Discord is the implementation of the day, but other frontends should be left open as a possible addition (Slack, IRC, etc)

## Observability
- We should have some centralized observability sink to collect standard prometheus/etc signals
- These signals should be able to drive dashboards and feed into other things (analytics, etc)
- Each component of the system should emit sufficient metrics, logs, etc to be able to tell what is going on with it,
  what might've gone wrong, how long things are taking, and so on
- A major consumer of the observability signals will be the bot that updates status and so on for the user

## Coding Focus: Initially
- Most of the focus in these design discussions has been around agents that ultimately produce code, but we should
think about this in a way that leaves the door open for agents that do other things, like research and assemble
recipes, or watch for plane ticket price fluctuations, etc.


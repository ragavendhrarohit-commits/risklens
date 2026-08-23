# RiskLens Architecture Course

This folder is a guided course, not a dump of file comments.

**Rule:** one feature at a time. Read the lesson, answer the knowledge check in chat, then we continue.

| Lesson | Document | Status |
|--------|----------|--------|
| 1 | [Application Activity](01-application-activity.md) | Start here |
| 2 | Sessions | After you pass Lesson 1 |
| 3 | Browser Activity | After Sessions |
| 4 | Users / Accounts | Later |
| 5 | Software (Installed Applications) | Later |
| 6 | Devices / Inventory | Later |
| 7 | Fleet | Later |
| 8 | Global Time Filter | Later |

Application Activity is first because it is the live usage pipeline: Windows foreground window → agent → API → PostgreSQL → Fleet table. Sessions are a dependency of that pipeline. Browser Activity is a *view* of Application Activity, not a second collector.

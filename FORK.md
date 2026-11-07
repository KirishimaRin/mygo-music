# What this fork adds

This fork of [egoist/mygo-music](https://github.com/egoist/mygo-music) adds the following to what a plugin can do. Everything else is as upstream's [README](README.md) says.

## A sidebar entry for a Home section

A Home section's `id`, the same each time `home` answers, lets `navigation` open that section's page of songs, as the card of the day's songs does. Give the entry the section's `id` as `section`. A `section` entry loads Home if it hasn't yet. Given a `page` as well, it opens the section's page, and versions of the app from before `section` open the `page`.

## A station of its own

A station with `standalone: true` leaves the switch for a banner of its own under the radio's card, the width of the page, which starts it by its `id` as any other and, while it plays, offers to leave it. Paused, it offers to resume. The rest go by the same rule as upstream.

The banner shows the station's `imageUrl`, or else its `icon`, one of the names `navigation` takes (`heart` makes the banner pink). Its title is the `headline`, under the `name` as a heading, or the `name` alone without one, and the `description` goes under the title.

## How long a song was heard

| Function | Parameters | Returns |
| --- | --- | --- |
| `playEnded` | `{id, seconds, reason}`, where `reason` is `ended`, `skipped`, `stopped` or `quit` | |

`played` is called once as each play of a song starts, and `playEnded` once as it ends: `seconds` is how much of it was heard, without the time it was paused or waited for its audio, or what a seek jumped over, and with what a seek back played again. `reason` is `ended` once it played to its end, `skipped` once something else plays, or the same song again, `stopped` once a remote plugin's song stopped at the other end, and `quit` as the app quits, which waits up to two seconds for the plugin to answer. Played again, a song is a new play, counted from nothing. A remote plugin's position is the app's guess from the clock and `remoteState`, so its `seconds` are only as close as that.

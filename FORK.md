# What this fork adds

This fork of [egoist/mygo-music](https://github.com/egoist/mygo-music) adds the following to what a plugin can do. Everything else is as upstream's [README](README.md) says.

## Releases

The [Addons Release](.github/workflows/addons-release.yml) workflow, run by hand from the Actions tab or with `gh workflow run addons-release.yml`, builds the apps of macOS, Windows and Linux and publishes them as a release of this fork. Its version is upstream's latest release with `-addons` after it, such as `0.1.21-addons`, and `0.1.21-addons.2` for a later build on the same upstream version. Installed apps update from this fork's releases, which are signed with its own key, not upstream's. The Mac app is signed ad hoc and not notarized, so macOS asks before it first opens.

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

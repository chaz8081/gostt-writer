# gostt-guess

A talking guessing game for a child. The player thinks of something, the
device asks yes-or-no questions out loud, listens to the spoken answers, and
guesses what it is. Every correct guess is added to a collection the player
can try to complete.

It is built from what the `gostt-pad` work showed: a Raspberry Pi runs a
real speech recognition model fast enough, entirely on the device. Nothing
is sent anywhere, and no network is needed after installing.

Status: the game, the voice and the hearing are tested on a Linux PC with
the real models, including whole spoken games through a simulated microphone
and speaker. **It has not yet run on a Raspberry Pi, and it has not been
tried with a child's voice.** See
[What has and has not been tested](#what-has-and-has-not-been-tested).

## What it fixes

| Problem with the shop-bought toy | What this does |
|---|---|
| Too quiet | A 3 W amplifier and a 4 ohm speaker, with a volume setting. |
| Does not understand the child | A full speech recognition model, which understood 98% of real recordings of a lone "yes" or "no" and never confused the two. "Yeah", "nope", "I don't think so", "not really" and "uh huh" all count. |
| A misheard answer ruins the game | Answers are weighed, not obeyed. One wrong or misheard answer lowers a candidate's score instead of ruling it out, and the device keeps asking after a wrong guess. |
| No way round the microphone | Three big buttons (yes, no, not sure) work at any moment, even while the device is still speaking. |
| Unclear when to talk | A light is on exactly while the device is listening. It does not listen while it speaks. |
| Stuck on unclear answers | It says it did not catch that and asks again, and "say that again" repeats the question. After three tries with no reply it stops and waits for the play button. |

The player can also say "I don't know", "start over" and "stop".

## What you need to supply: the pack

The things that can be guessed, and the questions that tell them apart, live
in one YAML file called a pack. The repository includes a sample pack of 40
animals (`packs/animals.yaml`), which is what the tests use.

**No Pokémon pack is included, and nothing here uses the toy's name, shell
design, artwork, voice lines or sounds.** Those belong to their owners, so
the character list is yours to write for your own family's use. The format
is short:

```yaml
name: My pack
thing: a creature            # spoken as "Think of a creature"
questions:
  - {id: flies, ask: "Can it fly?"}
  - {id: fire,  ask: "Is it a fire type?"}
items:
  - name: Example            # used in the collection
    say: Example             # how the name is spoken in "Is it ...?"
    yes: [flies, fire]       # questions that are true
    maybe: []                # true only sometimes, or open to argument
```

Anything not listed for an item is false. Check a pack before using it:

```bash
gostt-guess pack my-pack.yaml
```

It refuses a pack in which two items cannot be told apart, naming the pair,
and reports how many questions a round takes. Then play it by typing:

```bash
gostt-guess -text -pack my-pack.yaml
```

Both commands run on any computer with Go; they need no models or audio.
As a rough guide, not yet tried: about 150 items want 30 to 40 questions,
and a round should then take around 8 to 12 of them; the `pack` command
gives the real figure. If the voice says a name badly, spell `say` the way
it sounds.

## Hardware

The full build is in [`hardware/README.md`](hardware/README.md): the parts
list with links, the wiring, the printable case and the assembly steps. In
short: a Raspberry Pi 4 on a PiSugar 3 Plus battery board, a 2.8 inch touch
screen, a 3 W I2S amplifier and enclosed speaker, an I2S microphone and
five keyboard switches, in a hinged printed case with a magnetic lid.

On a Pi 4 or 5, a USB speakerphone and no buttons is enough for a first
try of the software: `gostt-guess -buttons=false`.

## Install

On the Pi:

```bash
sudo apt-get update && sudo apt-get install -y git
git clone -b claude/bold-carson-1o6fre https://github.com/chaz8081/gostt-writer.git
cd gostt-writer/guess
./deploy/install.sh
```

The installer adds system packages, Go, the two models (about 130 MB), the
program in `/opt/gostt-guess`, the settings and the sample pack in
`/etc/gostt-guess`, the voice clips, and a service that is installed but not
started. It is safe to run again.

To use your own pack, copy it to `/etc/gostt-guess/pack.yaml` and run
`gostt-guess voice` to make its clips.

## Test it, one part at a time

1. **The game, typed.** `gostt-guess -text`. Think of an animal and type
   your answers.
2. **Speaker.** `gostt-guess voice -say "Hello, can you hear me?"`. If it is
   too quiet, raise `audio.volume` in `/etc/gostt-guess/config.yaml`.
3. **Microphone and hearing.** `gostt-guess check`. It speaks a line, then
   records for three seconds: stay quiet for the first second, then say
   "yes". Expect `ok` lines for the speaker, the microphone and
   `hearing heard "Yes"`. It tells you when to change `audio.gain`.
   (The device's own synthetic voice is not a good test of hearing: models
   mishear its lone "yes". Use a person.)
4. **A spoken game without buttons.** `gostt-guess -buttons=false -debug`.
   A game starts at once. The log shows what was heard for each reply.
5. **Buttons.** `gostt-guess check -buttons 20s`, and press each button once.
6. **Start at boot.** `sudo systemctl enable --now gostt-guess`, and watch
   it with `journalctl -u gostt-guess -f`.

Have the child play step 4 with `-debug` on. Lines such as
`heard text="Nove." reply=no` show exactly what the model made of each
answer; anything that comes out as `reply=unclear` is worth reporting, since
the list of accepted words is in `internal/game/reply.go` and easy to
extend.

## If something fails

| Symptom | Likely cause |
|---|---|
| Replies register late, or only after a long pause | The room is noisy. Raise `listen.level`, or move the microphone away from the speaker. |
| The device answers itself | The microphone hears the speaker's tail. Raise `listen.skip_ms`. |
| Quiet answers are missed | Raise `audio.gain`, or lower `listen.level`. |
| A pause before some lines | Their clips are not made yet. Run `gostt-guess voice`. |
| Too many questions | Lower `game.confidence`. Too many wrong first guesses: raise it. |
| `arecord` or `aplay`: no such device | Wrong `-D` device; see `arecord -l` and `aplay -l`. |
| Buttons: permission denied | Log out and in again so the new group membership applies. |

## What has and has not been tested

Tested on a Linux PC:

- The unit tests (`go test ./...`): question choice, tolerance of wrong
  answers, reply parsing, the collection, the conversation (asking again,
  voice commands, buttons interrupting speech, going idle), deciding when a
  reply has ended, settings and audio files.
- Every item in the sample pack is guessed first time when answered
  truthfully (6.2 questions on average, 9 at most), and all 160 simulated
  rounds with one wrong answer still end on the right animal.
- Hearing, on real recordings: 824 clips of many different people saying a
  lone "yes" or "no" (the Google Speech Commands test set). 807 were
  understood (97.9%), none was heard as the opposite answer, 5 were not
  heard at all and 12 were unclear; the device asks again in both of those
  cases. Run it with `GOSTT_GUESS_WORDS_DIR`, see `internal/speech/models_test.go`.
- Hearing, on longer replies ("I don't think so", "say that again") spoken
  by the device's own voice: all 9 phrasings pass
  (`GOSTT_GUESS_ASR_DIR=... GOSTT_GUESS_VOICE_DIR=... go test ./internal/speech/`).
- The `gostt-guess` program itself playing a spoken game against a scripted
  player, through stand-in microphone and speaker commands, using real
  "yes" and "no" recordings: 16 of 18 replies understood. The other two
  were the same quiet recording, which was not heard, so the question was
  asked again. A reply is taken as finished after 0.6 seconds of silence
  (`listen.quiet_ms`).
- The installer's download, settings and service steps, into a scratch
  directory.

Not tested:

- **Your child's voice, in your house.** The recordings above are mostly
  adults, one word each, with little background noise. Speech models are
  generally worse on young children, and how much worse here is unknown.
  The buttons are the fallback.
- Anything on a Raspberry Pi: speed, the ARM64 build, the I2S sound card,
  the buttons and light, and how long making the clips takes.
- The microphone hearing the device's own speaker in a real enclosure.

## Not built yet

- Anything on the screen. The hardware build includes one, wired and
  mounted, but the program does not draw on it yet.
- Sound effects, and a pause before the guess.
- Learning a new item when the device gives up.
- Languages other than English.

## Layout

| Path | Contents |
|---|---|
| `cmd/gostt-guess` | The program: play, `pack`, `voice`, `check` |
| `internal/game` | Packs, question choice, reply parsing, the collection. No hardware. |
| `internal/session` | The conversation, behind a small speak/hear interface |
| `internal/speech` | Speech recognition, the voice and its clip cache, end-of-reply detection |
| `internal/audio` | Capture and playback commands, WAV files |
| `internal/gpio` | Buttons and light |
| `internal/config` | Settings file |
| `packs` | The sample pack |
| `deploy` | Installer and service file |

This is its own Go module, like `pad/`, so it builds on a Pi without the
macOS app's dependencies. The audio capture, GPIO and model-loading code
started as copies of the pad's, because the pad branch is not merged; if
both land, they can share one package. Speech recognition is Kroko's English
streaming Zipformer (CC-BY-SA 4.0, by Banafo) and the voice is a Piper voice
(see the `MODEL_CARD` file in its folder), both run with sherpa-onnx
(Apache-2.0).

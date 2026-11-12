# On-device speech-to-text on the ESP32-S3: model survey

Date: 2026-11-12
Status: research plus a host-side run of Oído (see "Host evaluation"). Nothing
has been run on hardware yet.

## Question

Can the ESP32-S3 boards already used for the BLE keyboard dongle do the
transcription themselves, so the device is a standalone "speak, and it types"
USB keyboard with no Mac, no server and no network?

## Short answer

Yes, as of the last six weeks. Until October 2026 the honest answer was no:
an ESP32-S3 could do wake words and fixed command lists (Espressif ESP-SR /
MultiNet) and everything open-vocabulary went to a server. Several projects
have since shown open-vocabulary recognition running entirely on an
ESP32-S3 N16R8, which is the board this repo already targets.

One of them, **Oído v2** (Lokutor, 3 Nov 2026), is measured faster than real
time on that board with partial text while speaking. It is the only candidate
that meets "real time" on the S3 today, and it is the recommended starting
point. Its limits are real: English only, lowercase text without punctuation,
accuracy well below the Mac backends on noisy speech, a non-commercial model
licence, and a GPLv3 engine.

## Target hardware

| Item | Value | Source |
|---|---|---|
| Board | Lonely Binary ESP32-S3 N16R8 (ESP32-S3-WROOM-1 class) | `firmware/esp32/README.md` |
| Flash / PSRAM | 16 MB / 8 MB octal | same |
| CPU | 2 cores, 240 MHz, PIE SIMD, single-precision FPU, no NPU | Espressif |
| USB | UART port for flashing, native USB-OTG for HID | same |
| Microphone | none on board; an I2S MEMS mic must be added | |

The Amazon short link in the request could not be opened from this session
(`a.co` is not on the network allowlist). The board above is the one the repo
documents as tested; **confirm it is the same listing**. What matters is
N16R8: every candidate below needs 16 MB flash and 8 MB PSRAM.

## Candidates

All figures are the projects' own. "Measured" means on a physical S3 board.

| Project | Model | Flash | Speed on S3 (RTF, lower is faster) | LibriSpeech WER clean / other | Streaming | Licence (code / weights) |
|---|---|---|---|---|---|---|
| **Oído v2 A** | Conformer-CTC, int4, + 1.3 MB LM | 7.7 + 1.3 MB | **0.72 measured**, 1 core | 7.1 / 16.4 streaming; 4.6 / 10.7 full context | yes, partial text | GPL-3.0 / CC BY-NC-SA 4.0 |
| **Oído v2 B** | smaller variant, int4, + LM | 5.9 + 1.3 MB | **0.54 measured**, 1 core | 10.6 / 22.4 streaming; 6.2 / 14.4 full context | yes, partial text | same |
| Oído v1 `nemo8` | Conformer-CTC Small (16 layers, d=176), int8, + LM | 14.0 + 1.3 MB | 1.97 measured (2x slower than speech) | 3.3 / 7.2 | no, text after utterance | same |
| Oído v1 `nemo4` | same, int4 | 8.3 MB | 1.76 measured | 4.6 / 10.0 greedy | no | same |
| whistle-esp32 | Cactus Whistle, decoder cut to depth 4, int8 | 14.5 MB | ~3.2–3.9 measured (2 s command takes 6.3 s) | 11.7 on 73 utterances | no | community port / Apache-2.0 |
| TinySTT | conv + 2xGRU phoneme CTC (0.9 M params) + lexicon beam search | 3.1 or 9.5 MB | 0.43 + 0.014 measured | 29.6 / n/a (25.7 large) | yes | Apache-2.0 |
| ESP-SR MultiNet7 | fixed command list (200–300 phrases) | 2.7 MB | real time | n/a (not open vocabulary) | n/a | Espressif |

For scale, the same normalisation gives Moonshine tiny 5.0 / 12.1 and Whisper
tiny.en 6.3 / 15.9 on a laptop (Oído's comparison table). So Oído v1 beats
Whisper tiny on accuracy but is too slow, and v2 A in streaming mode lands
slightly behind Whisper tiny while running live on a $5 chip.

### Oído v2 in more detail

Read directly from `lokutor-ai/oido` at commit `8c3d78e` (11 Nov 2026).

- Board used for measurement: ESP32-S3-WROOM-1-N16R8, one core, 240 MHz.
- Final text arrives 431 ms (A) or 259 ms (B) after speech ends, measured with
  audio fed at speaking pace. The firmware's end-of-speech detector adds
  0.8 s of silence by default (`CONFIG_TASR_SEG_HANG_MS`), so in practice
  about 1.1–1.3 s. That wait is tunable.
- The figures above use 0.64 s chunks. 1.28 s chunks improve variant A to
  6.0 / 14.0 WER at RTF 0.61, with ~660 ms final-text delay.
- The board and host builds print identical transcripts on 18 of 18 clips, so
  the engine can be developed and tested on a laptop.
- The engine API is small and fits our firmware style: `tasr_model_load`,
  `tasr_stream_create`, `tasr_stream_feed(pcm, n)`, `tasr_stream_finish`,
  `tasr_stream_text`. Portable C99 with optional S3 SIMD kernels.
- All model files are in the repo; nothing has to come from Hugging Face.

Caveats, all stated by the authors:

- **The live microphone path is not measured.** Their test board had no mic.
  They estimate +0.015 RTF for the mic buffers. This is the first thing we
  would have to prove on our hardware.
- **Dual-core is broken** on their boards (wrong transcripts, brownouts under
  dense SIMD load on both cores; they suspect the 3.3 V rail). One core is
  the supported path. That leaves the second core for USB HID and audio
  capture, which suits us, but power supply quality matters for the build.
- A one-shot `transcribe` inside a live capture loop hangs on the chip for
  unknown reasons. Streaming works. Use streaming.
- English only for v2. A Spanish v1 model exists (not real time).
- Output is lowercase with no punctuation.
- Noisy speech stays above 10 % WER in streaming mode.
- 49 commits, no tagged releases. It is six weeks old.

### Why the others are not the pick

- **Whistle** is the more capable model (7 languages, punctuation) but takes
  three to four times real time on the S3 and is not streaming. The official
  engine source is not published; the S3 port is a community rewrite.
- **TinySTT** is fast and permissively licensed, but 26–30 % WER is not
  dictation quality. Its author says the same. Worth watching.
- **ESP-SR** is not open vocabulary.
- **Moonshine** has a microcontroller line (Moonshine Micro, RP2350 demo) but
  the demo recognises letters, digits and a few commands. Moonshine tiny
  (27 M params) and streaming tiny (34 M) have no S3 port and are several
  times larger than anything shown to run on this chip.
- **sherpa-onnx / zipformer**: no microcontroller port found.

### If the S3 turns out not to be enough

`esp32-pie-asr` runs a 65.7 M parameter streaming Zipformer transducer on an
**ESP32-P4** at RTF 0.70 with 4.3 / 10.3 WER, Apache-2.0 code and CC-BY-SA
weights. It needs the P4X function EV board (chip rev v3, 32 MB PSRAM) and a
microSD card for the weights, and words finalise about 2 s late. That is a
different board and a different build, but it is the clear upgrade path:
better accuracy and commercially usable licences.

## What this means for the project

A standalone device changes the architecture more than the model does:

| Today | Standalone |
|---|---|
| Mac records and transcribes | I2S mic on the ESP32 |
| Text sent over encrypted BLE | no radio needed |
| ESP32 types via USB HID | ESP32 types via USB HID (reuse `usb_hid.c`) |

- `usb_hid.c`, `led.c` and the mute handling carry over. BLE, pairing and
  the protobuf layer are not needed for the standalone path. Dropping NimBLE
  also frees internal RAM, which is the scarce resource: Oído's v1 firmware
  reports only 74–128 KB internal RAM free at its low-water mark.
- The partition table must change. It is currently a single 1 MB app; the
  model needs its own ~9 MB data partition.
- A push-to-talk key maps naturally onto the keyboard switches planned for
  the build, and sidesteps the 0.8 s silence wait: release the key, flush.
- Partial text needs a policy. A streaming recogniser revises its guess; a
  keyboard cannot cheaply un-type. Either type only finalised text, or type
  partials and send backspaces on revision.
- No punctuation or capitalisation comes from the model. A small rule-based
  pass (sentence case, spoken "comma" / "period") is the realistic option.

### Licensing, which bears on the "own project or supersede" decision

- This repo is MIT. Oído's engine is **GPL-3.0**: firmware that links it must
  be GPL-3.0 as a whole. A separate project, or a clearly separated
  GPL-licensed firmware directory, is cleaner than mixing it into the MIT
  tree.
- Oído's weights are **CC BY-NC-SA 4.0** plus terms that forbid commercial
  use, including selling devices, without a written licence. Fine for a
  personal build. Lokutor says it offers no-cost licences to small makers.

## Recommended next steps

1. **Host evaluation.** Done for Oído, see below. Still open: run
   `task bench` on the Mac for the whisper and parakeet numbers on the same
   clips, and record a few minutes of real dictation to replace the TTS
   clips.
2. **Board bring-up without a mic.** Flash Oído's stock firmware in file mode
   on one of the boards and reproduce their RTF figures.
3. **Add a microphone.** INMP441 is what every project uses (SCK GPIO4,
   WS GPIO5, SD GPIO6, L/R to GND; no conflict with our LED on GPIO48 or BOOT
   on GPIO0). Measure the live path, which nobody has published.
4. **Integrate.** New firmware target: mic, Oído streaming engine on one
   core, existing USB HID typing on the other, push-to-talk on a GPIO.
5. Decide on project structure once step 3 shows whether accuracy on a real
   microphone is good enough to be worth a hardware build.

## Host evaluation (2026-11-12)

Oído's host CLI (`esp32/host/tasr_cli`, commit `8c3d78e`) built on Linux x86
and run on this repo's benchmark clips from
`internal/transcribe/testdata/references.json`, plus whisper.cpp's `jfk.wav`.
The authors report the host build and the chip print identical transcripts
for v2, so this should predict on-chip accuracy; that parity was not
re-checked here. Scoring: lowercase, punctuation stripped, word edit distance.

Word errors / reference words:

| Configuration | On-chip speed | short | medium | long | jfk | 3 tech clips | all 4 |
|---|---|---|---|---|---|---|---|
| v2 A, streaming 0.64 s, LM | 0.72, real time | 2/9 | 9/19 | 8/31 | 0/22 | 32.2 % | 23.5 % |
| v2 A, streaming 1.28 s, LM | 0.61, real time | 2/9 | 9/19 | 7/31 | 0/22 | 30.5 % | 22.2 % |
| v2 A, streaming 0.64 s, greedy | real time | 2/9 | 10/19 | 6/31 | 4/22 | 30.5 % | 27.2 % |
| v2 B, streaming 0.64 s, LM (beam 2) | 0.54, real time | 4/9 | 10/19 | 5/31 | 1/22 | 32.2 % | 24.7 % |
| v2 B, streaming 1.28 s, LM | real time | 2/9 | 10/19 | 5/31 | 2/22 | 28.8 % | 23.5 % |
| v2 A, full context, LM | not real time | 1/9 | 7/19 | 3/31 | 0/22 | 18.6 % | 13.6 % |
| v2 B, full context, LM | not real time | 2/9 | 7/19 | 5/31 | 0/22 | 23.7 % | 17.3 % |
| v1 nemo8, LM | 1.97, 2x too slow | 1/9 | 7/19 | 5/31 | 0/22 | 22.0 % | 16.0 % |
| v1 nemo4, greedy | 1.76, too slow | 1/9 | 6/19 | 3/31 | 0/22 | 16.9 % | 12.3 % |

What v2 A streaming (the real-time setting) actually printed:

| Reference | Output |
|---|---|
| The kubernetes cluster ran three pods on each node. | that cuber's cluster ran three pods on each node |
| The webhook endpoint returned a four hundred error because the payload exceeded the maximum size limit for JSON serialization. | the we would then point returned to four hundred a because the paload exceeded the maximum size limit for jason ceallization |
| After deploying the micro services to production we noticed that the gRPC connections were timing out. The root cause was a misconfigured load balancer that did not support HTTP two multiplexing. | after deploying the micro's services to production we noticed that the grpc connections were timing out the route cost was a misconigured low balancer that did not support age d pto multiplexing |
| And so my fellow Americans ask not what your country can do for you ask what you can do for your country. | and so my fellow americans ask not what your country can do for you ask what you can do for your country |

Reading the result:

- **Plain English is fine.** JFK is word-perfect in every LM configuration.
- **Technical dictation is not.** Roughly one word in three is wrong in the
  real-time setting. Most errors are jargon the model has never seen
  (kubernetes, webhook, endpoint, payload, JSON, HTTP). Some are not:
  "error" became "a", "root cause" became "route cost", "load" became "low".
  Those ordinary-word errors mostly disappear with full context, so they are
  the cost of streaming.
- **The model emits non-words** ("paload", "misconigured", "ceallization").
  It spells from sub-word pieces; there is no dictionary constraint.
- **The slow v1 model does not rescue jargon.** It is better on ordinary
  words but still fails on the same vocabulary, so this is a training-data
  limit and not a quantisation or streaming artefact.
- Variant B is not clearly worse than A on these clips, and 1.28 s chunks
  help only slightly. The sample is too small to rank them.

Limits of this run: four clips and 81 words, three of them one macOS TTS
voice reading sentences chosen to be hard. This is directional, not a
benchmark. **The whisper and parakeet baselines were not run**: both backends
need macOS (Metal / CoreML) and this ran on Linux. Run `task bench` on the
Mac to fill in that column; expect a large gap in their favour on jargon.

Implication: as it stands this is a standalone dictation device for everyday
English, not a replacement for the Mac app for developer dictation. A
vocabulary-biasing step (boosting a user word list in the on-chip beam
search) or fine-tuning on technical speech would be the way to close that
gap, and the licence terms cover derived models too.

To reproduce:

```
git clone https://github.com/lokutor-ai/oido && cd oido/esp32/host && make tasr_cli
./tasr_cli ../../models/oido_v2a.tnm --lm ../../models/oido_v2.tlm --nstream --chunk 8 clip.wav   # streaming 0.64 s
./tasr_cli ../../models/oido_v2a.tnm --lm ../../models/oido_v2.tlm clip.wav                       # full context
```

## Larger models on the same clips (2026-11-12)

To see whether better hardware would fix the jargon problem, the models that
such hardware could run were tested on the same four clips. All ran on Linux
x86 through sherpa-onnx 2.0.1, using the model archives from the sherpa-onnx
`asr-models` GitHub release. This measures accuracy only. Nothing here says
how fast any of them runs on a given board.

Scoring differs slightly from the table above: numbers and compounds are
normalised on both sides ("400" = "four hundred", "HTTP2" = "HTTP two",
"micro services" = "microservices", "web hook" = "webhook"), because these
models write digits. Oído is re-scored the same way, so rows are comparable
within this table. Tech clips total 58 words after normalisation.

| Model | Size on disk | Smallest hardware that could run it | Streaming | Tech clip errors | JFK errors |
|---|---|---|---|---|---|
| Parakeet TDT 0.6B v2, int8 | 661 MB | Pi 5 class | no | **0 / 58** | 0 |
| Kroko streaming Zipformer (65.7 M) | 71 MB | **ESP32-P4X** (as 2/4-bit, 27 MB) | yes | **1 / 58 (1.7 %)** | 0 (beam), 1 (greedy) |
| Moonshine base (v1 int8, v2 quantised) | 141–286 MB | Pi class | no (as run) | 1 / 58 (1.7 %) | 0 |
| Moonshine tiny v2, quantised | 44 MB | Pi class | no (as run) | 2 / 58 (3.4 %) | 0 |
| Whisper base.en, int8 | 160 MB | Pi class | no | 2 / 58 (3.4 %) | 0 |
| Moonshine tiny v1, int8 | 124 MB | Pi class | no | 4 / 58 (6.9 %) | 0 |
| Whisper tiny.en, int8 | 103 MB | Pi class | no | 5 / 58 (8.6 %) | 0 |
| Oído v1 nemo4 (too slow on S3) | 8.3 MB | ESP32-S3 | no | 10 / 58 (17.2 %) | 0 |
| Oído v2 A, full context (not real time) | 9.0 MB | ESP32-S3 | no | 11 / 58 (19.0 %) | 0 |
| Oído v2 A, streaming 0.64 s (real time) | 9.0 MB | ESP32-S3 | yes | 20 / 58 (34.5 %) | 0 |

What they printed for the hardest clip (reference: "The webhook endpoint
returned a four hundred error because the payload exceeded the maximum size
limit for JSON serialization."):

| Model | Output |
|---|---|
| Parakeet | The webhook endpoint returned a 400 error because the payload exceeded the maximum size limit for JSON serialization. |
| Kroko | The web hook endpoint returned a four hundred error because the payload exceeded the maximum size limit for JSON serialization. |
| Moonshine tiny v2 | The webhook endpoint returned a 400 error because the payload exceeded the maximum size limit for JSON serialization. |
| Moonshine base | The Wethhook endpoint returned a 400 error because ... JSON serialization. |
| Whisper base.en | The wet hook endpoint returned a 400 error because ... JSON serialization. |
| Oído v2 A streaming | the we would then point returned to four hundred a because the paload exceeded the maximum size limit for jason ceallization |

Findings:

- **The jargon problem is a model-size problem, and it goes away one step
  up.** Every model here beats every Oído configuration by a wide margin.
  All of them also produce capitalisation and punctuation, which Oído does
  not.
- **Kroko is the standout for this project.** It is a true streaming model,
  made one error in 58 words ("HTTP two" heard as "HTTP to"), and is the
  model `esp32-pie-asr` runs in real time on an ESP32-P4X. It matched the
  600 M parameter Parakeet to within one word.
- "Kubernetes" separates the tiers: Parakeet, Kroko, Moonshine base and
  Whisper base get it; both tiny models and every Oído variant do not.
- Moonshine tiny v2 is good for 44 MB, better than Whisper base.en here.

Caveats:

- Still four clips, three from one TTS voice. The ranking between the top
  five models is noise; the gap between them and Oído is not.
- **Kroko was tested as released (float ONNX), not the 2/4-bit weights the
  P4 runs.** The P4 project reports that requantisation costs about half a
  point of LibriSpeech WER (3.94 to 4.34 clean). The 2/4-bit weights were
  tested afterwards; see "The P4's compressed weights" below.
- Kroko needed about 2 s of trailing silence before it emitted the last
  word; with 0.8 s the final word was cut off. This is consistent with the
  roughly 2 s word latency the P4 project reports, and matters for a
  push-to-talk design: keep feeding silence after key release.
- Parakeet here is the int8 ONNX export, standing in for the CoreML build the
  Mac app uses. Whisper is the ONNX export, not whisper.cpp.
- Moonshine v2's streaming tiny/small/medium archives could not be located
  (Hugging Face is blocked from this session); the v2 tiny and base tested
  were run on whole utterances.
- A 20 M parameter streaming Zipformer was also run and dropped the first
  words of every clip. That looks like a harness problem and it is excluded.

### The P4's compressed weights (2026-11-12)

The P4 project's engine (`esp32-pie-asr`, commit `68fcf81`) was built for the
host (`pieasr/host`) and run with the actual 2/4-bit weights the board loads
(`kroko-en-q24-v1.bin`, 26.7 MB, SHA-256 verified against the repo's pinned
hash). The engine's golden test passes: 8 clips, 0 differences from the
recorded reference outputs. The authors state the host build computes what
the chip computes bit for bit; that claim was not checked on a board.

| Configuration | Tech clip errors (58 words) | JFK errors |
|---|---|---|
| `int+beam4`, the firmware's configuration | 1 (1.7 %) | 0 |
| `int`, greedy | 1 (1.7 %) | 0 |
| `int+beam4` with a 5-entry phrase list | **0** | 0 |
| For reference: Kroko as released, float (above) | 1 (1.7 %) | 0 |

- **Compression cost nothing on these clips.** The 27 MB weights made the
  same single error as the 71 MB original: "HTTP two" heard as "HTTP to".
- **Phrase biasing fixed that error.** The engine takes a text file of words
  to listen for (`PIEASR_PHRASES`). With `HTTP/2 = HTTP two`, `gRPC = GRPC`,
  `Kubernetes`, `webhook`, `JSON`, the long clip came out as "HTTP two
  multiplexing" and nothing else broke. The list was written after seeing
  the error on this same clip, so this shows the mechanism works, not how
  well it generalises. The host tool only biases the search; rewriting the
  spoken form to the shown form ("HTTP two" to "HTTP/2") happens in the
  firmware and was not exercised.
- Capitalisation at sentence starts is inconsistent ("the Kubernetes
  cluster"), and biasing capitalised one word oddly ("webhook Endpoint").
  A small clean-up pass would still be needed.
- Each clip had 2 s of silence appended, as in the earlier Kroko run.
- Host run times say nothing about the board; the C reference kernels are
  slow on purpose.

### Revised hardware recommendation

The ESP32-S3 with Oído is real, but not good enough for technical dictation.
The **ESP32-P4X with 32 MB PSRAM** is now the first choice: it keeps the
microcontroller advantages (instant boot, one USB cable, existing HID
firmware carries over) and the model it runs was near-perfect on these
clips. Licences are also cleaner: Apache-2.0 engine, CC-BY-SA weights. Open
risks: the ~2 s word latency (not tested), real-microphone audio (not
tested), the microSD requirement, and which retail boards carry the required
chip revision (v3 / P4X). A Pi-class board remains the fallback if the P4
disappoints.

## Verification notes

- Read directly from a clone: Oído's README, engine header, model licence and
  terms, `results/board_esp32s3.json`, `results/v2/numbers.json`.
- Read from the project pages on GitHub: whistle-esp32, TinySTT,
  esp32-pie-asr, the Oído README.
- From search summaries only, not opened (hosts blocked in this session):
  the Oído arXiv paper (2610.13447), Hugging Face model cards, the Cactus
  Whistle announcement, the TinySTT blog post, Moonshine parameter counts.
- Run here: Oído's host CLI, and the sherpa-onnx models in the section
  above. Accuracy only. Speed, memory and board figures are the authors' and
  have not been reproduced.

## Sources

- https://github.com/lokutor-ai/oido
- https://arxiv.org/abs/2610.13447
- https://huggingface.co/lokutor-ai/oido-v2-realtime-int4
- https://github.com/andrisgauracs/whistle-esp32
- https://huggingface.co/Cactus-Compute/whistle
- https://github.com/pschatzmann/TinySTT
- https://www.pschatzmann.ch/home/2026/10/27/tinystt-speech-to-text-on-a-microcontroller-a-proof-of-concept/
- https://github.com/BoumedineBillal/esp32-pie-asr
- https://github.com/moonshine-ai/moonshine/blob/main/micro/examples/rp2350/README.md
- https://huggingface.co/UsefulSensors/moonshine-streaming-tiny
- https://docs.espressif.com/projects/esp-sr/en/latest/esp32s3/
- https://github.com/k2-fsa/sherpa-onnx/releases/tag/asr-models
- https://huggingface.co/BoumedineBillal/esp32-pie-asr-kroko-en

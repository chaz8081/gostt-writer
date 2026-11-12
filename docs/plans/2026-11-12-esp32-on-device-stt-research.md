# On-device speech-to-text on the ESP32-S3: model survey

Date: 2026-11-12
Status: research only. Nothing here has been built or run on hardware yet.

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
- 0.64 s chunks instead of 0.32 s improve variant A to 6.0 / 14.0 WER at RTF
  0.61, with ~660 ms final-text delay. Likely the best dictation setting.
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

1. **Host evaluation.** Build Oído's host CLI and run variants A and B
   against the same recordings used for this repo's whisper/parakeet
   benchmarks, at 0.32 s and 0.64 s chunks. This needs no hardware and gives
   a direct accuracy comparison with what the Mac app does today.
2. **Board bring-up without a mic.** Flash Oído's stock firmware in file mode
   on one of the boards and reproduce their RTF figures.
3. **Add a microphone.** INMP441 is what every project uses (SCK GPIO4,
   WS GPIO5, SD GPIO6, L/R to GND; no conflict with our LED on GPIO48 or BOOT
   on GPIO0). Measure the live path, which nobody has published.
4. **Integrate.** New firmware target: mic, Oído streaming engine on one
   core, existing USB HID typing on the other, push-to-talk on a GPIO.
5. Decide on project structure once step 3 shows whether accuracy on a real
   microphone is good enough to be worth a hardware build.

## Verification notes

- Read directly from a clone: Oído's README, engine header, model licence and
  terms, `results/board_esp32s3.json`, `results/v2/numbers.json`.
- Read from the project pages on GitHub: whistle-esp32, TinySTT,
  esp32-pie-asr, the Oído README.
- From search summaries only, not opened (hosts blocked in this session):
  the Oído arXiv paper (2610.13447), Hugging Face model cards, the Cactus
  Whistle announcement, the TinySTT blog post, Moonshine parameter counts.
- Not done: no third-party code was built or run. No claim above has been
  reproduced independently.

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

# Tobi Dynavox Spellchecker

A portable Windows 10/11 x64 proofreading tool for people composing their own AAC messages. It defaults to the installed Windows English (US) dictionary, which needs no account, model download, or internet connection. Six optional models run on your PC after a one-time download. It does not record drafts. Windows and the six local models check drafts without sending them anywhere. With a configured cloud engine selected, pressing Spell Check sends the current draft to that provider. There is no background capture or sending. This is an independent tool, not a Tobii Dynavox product.

## What gets corrected

Every engine includes an offline pass for English sentence capitalization, the pronoun “I,” and some common agreement, article, and apostrophe mistakes. It runs even when the engine finds no misspellings. For example, “i has a appointment” becomes “I have an appointment.”

The Windows dictionary and local spelling models do not provide full contextual grammar checking. Select **DeepSeek** or **Grok** for broader grammar and punctuation. Automatic write-back allows small grammar repairs but rejects obvious rewrites or changes to numbers or negation. It leaves intentional short messages where rules do not apply.

## Spelling engines

Choose an engine in the app, then click **Check spelling** or capture a field as usual. Windows Spell Checking is selected by default. An explicitly chosen engine is remembered for the next time you open the app or use the AAC button. The first use of an optional local model automatically sets up private CPU inference libraries and downloads its weights. No separate Python installation or administrator rights are needed. Downloads are cached under `%LOCALAPPDATA%\AAC Spellchecker`. The EXE embeds the six engine adapters, not their multi-gigabyte model weights. After setup, that engine works offline. Drafts are never sent to the download providers.

| Engine | Suggested PC RAM | Notes |
| --- | --- | --- |
| Windows Spell Checking (default) | 8 GB or 16 GB | Uses the installed English (US) Windows dictionary. No download. |
| [NeuSpell BERT](https://huggingface.co/murali1996/bert-base-cased-spell-correction) | 8 GB or 16 GB | Uses sentence context for word-level corrections. About 744 MB of model files. |
| [NeuSpell SC-LSTM](https://github.com/neuspell/neuspell) | 8 GB or 16 GB | Uses letters and context to suggest word-level corrections. About 452 MB. |
| [BART English Spelling Correction](https://huggingface.co/oliverguhr/spelling-correction-english-base) | 8 GB or 16 GB | Experimental text-generation model. Can change punctuation or wording. About 561 MB. |
| [T5-large-spell](https://huggingface.co/ai-forever/T5-large-spell) | 16 GB recommended | Generates a corrected version of the input. About 3 GB. |
| [SAGE mT5 Large](https://huggingface.co/ai-forever/sage-mt5-large) | 16 GB recommended | Large multilingual model with English support. About 4.9 GB. |
| [Spell Correct Transformer](https://huggingface.co/torinriley/spell-correct-transformer) | 8 GB or 16 GB | Experimental, one word at a time, no context. About 4 MB. |

These are suggested computer RAM classes, not compatibility guarantees from device benchmarks. The sizes are estimates for model files only. Allow additional disk space for the shared inference libraries. The large models may respond slowly on CPU. Review all model changes before applying them, especially with names, numbers or shorthand. Switch back to Windows Spell Checking if a download fails or you want to check a draft without waiting.

## Optional cloud spelling

Select **DeepSeek (cloud)** or **Grok (cloud)** in the desktop spelling-engine menu. Enter that provider’s API key in the masked box and choose **Save key**. Keys are encrypted for the current Windows user and can be deleted using **Remove key**. The app does not ship with a key, and each provider account bills its own API use.

- **DeepSeek** uses [DeepSeek Flash](https://api-docs.deepseek.com/api/create-chat-completion/) with thinking turned off.
- **Grok** uses [Grok 4.20 Non-Reasoning](https://docs.x.ai/developers/models/grok-4.20-non-reasoning) through the Responses API and requests `store: false`.

Once an engine is selected and its key is saved, pressing **Spell Check** in TD Snap checks the current draft using that engine. There is no separate cloud confirmation. The app does not watch the draft while you type or send previous drafts. Select **Windows Spell Checking** in the tray settings for offline checks. Cloud checks require internet and are subject to the provider’s data policies.

## Download

v0.5.0 is a device-testing preview. TD Snap write-back and gaze selection have not been verified on a physical device.

- [Download the portable Windows EXE](https://github.com/tbenitz/Tobi-Dynavox-Spellchecker/releases/download/v0.5.0/Tobi-Dynavox-Spellchecker.exe)
- [Download the source ZIP](https://github.com/tbenitz/Tobi-Dynavox-Spellchecker/releases/download/v0.5.0/Tobi-Dynavox-Spellchecker-source.zip)

## TD Snap setup for eye gaze

**Daily use:** compose in TD Snap, then dwell on **Spell Check** once. The spellchecker checks and replaces the linked draft in place without opening a review window, changing focus, or requiring another click. It never presses **Speak**. The spellchecker checks that the draft has not changed and rejects invalid output, changed numbers, and obvious rewrites. The latest status is available on the tray icon and in Settings without a popup.

**One-time setup** (a caregiver or setup partner can do this):

1. Save the EXE in a permanent folder on the Windows device and open it normally. Leave **Windows Spell Checking** selected, or choose another engine. For an optional cloud engine, enter your key and choose **Save key**.
2. In TD Snap, open the keyboard page and enter a plain-text sample. In Spellchecker choose **Link AAC field**, dismiss the instructions, and leave the pointer on the TD Snap Message Window until it is linked.
3. In TD Snap Edit mode, add a **Spell Check** button. Set its action to **Launch Application**, select the EXE, and enter `--check` in **Advanced → Command Line Arguments**. The launch with `--check` is the trigger each time. It starts the hidden tray app if needed or hands the request to the instance already running. Double-clicking the EXE without an argument opens Settings instead. If you already set up the button, quit the old spellchecker and replace the EXE at the same path. The button, link, and settings can stay as they are.
4. In normal TD Snap mode, test with an unsent draft such as “I wnat a glass of water.” Choose **Spell Check** once, then check that the corrected draft appears in the same Message Window. Speak only when you're ready.

**Settings and Undo:** Click the **AAC Spellchecker** icon next to the Windows clock to open Settings. It may be under the hidden-icons arrow. Closing the window leaves the spellchecker in the tray. The tray menu has **Undo last automatic correction** and **Exit**. For gaze-only Undo inside TD Snap, an optional second **Undo spelling** button can launch the same EXE with `--undo`. Undo restores the last draft only if it still matches the corrected text.

**Compatibility:** The TD Snap window must remain active during the check and its Message Window must expose a uniquely identifiable, writable plain-text field. TD Snap must also be allowed to launch the EXE. Some kiosk configurations block application launches. Direct editing has not yet been verified on Tobii Dynavox hardware. [TD Snap manual](https://download.mytobiidynavox.com/Snap/documents/TD_Snap_UsersManual_en-US.pdf)

### Other ways to use the tool

The regular EXE still has the optional **Capture field**, **Ctrl+Alt+F8**, and clipboard controls for people who want a general-purpose desktop spellchecker. **Undo AAC fix** in the regular window supports the last manual edit or automatic edit when available. These are separate from the gaze workflow.

The spellchecker never invokes Speak or sends a Speak shortcut. If AAC is configured to speak inserted text, the AAC app's own settings still apply.

## Requirements and test status

- Windows 10/11 x64, with the English (US) spelling dictionary installed. If the spelling service reports that the dictionary is missing, add English (United States) in Windows Language settings.
- Windows checks spelling, then the app adds basic English capitalization and grammar offline. DeepSeek and Grok provide broader contextual corrections. The TD Snap button applies an accepted correction automatically when the draft is still current.
- The linked-field automatic edit and Undo are tested with a Windows test app. The TD Snap launch action and direct write-back have not been verified on a physical Tobii Dynavox device. Test with a short unsent draft first, such as “I wnat a glass of water.”
- The executable is unsigned. Windows may display its usual warning for a new unsigned app. It does not need administrator privileges.

## Build from source

To build locally with Go 1.25.5 on 64-bit Windows:

```powershell
go test ./...
go build -buildvcs=false -trimpath -ldflags "-s -w -H windowsgui" -o Tobi-Dynavox-Spellchecker.exe .
```

Portable unit tests cover the engine catalog, model-preview validation, UTF-16 replacement offsets, stale-draft protection, read-back verification and undo, plus simulated DeepSeek and Grok requests. Windows tests verify encrypted, provider-specific key storage, tray API layout, linked-field automatic replacement, and Undo. The Windows CI smoke test provisions the runtime and checks one optional model before and after blocking downloads. Windows-only tests also exercise the system dictionary and read, replace and re-read a linked field in a separate Win32 test app. Live device testing is still needed for the TD Snap direct route.

# Tobi Dynavox Spellchecker

A portable Windows 10/11 x64 spelling review tool for people composing their own AAC messages. It defaults to the installed Windows English (US) dictionary, which needs no account, model download, or internet connection. Six optional models run on your PC after a one-time download. It does not record drafts or send them anywhere. This is an independent tool, not a Tobii Dynavox product.

## Spelling engines

Choose an engine in the app, then click **Check spelling** or capture a field as usual. Windows Spell Checking is selected by default. An explicitly chosen engine is remembered for the next time you open the app or use the AAC button. The first use of an optional engine automatically sets up private CPU inference libraries and downloads its weights. No separate Python installation or administrator rights are needed. Downloads are cached under `%LOCALAPPDATA%\AAC Spellchecker`. The EXE embeds the six engine adapters, not their multi-gigabyte model weights. After setup, that engine works offline. Drafts are never sent to the download providers.

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

## Download

v0.2.0 is a device-testing preview. TD Snap write-back and gaze selection have not been verified on a physical device.

- [Download the portable Windows EXE](https://github.com/tbenitz/Tobi-Dynavox-Spellchecker/releases/download/v0.2.0/Tobi-Dynavox-Spellchecker.exe)
- [Download the source ZIP](https://github.com/tbenitz/Tobi-Dynavox-Spellchecker/releases/download/v0.2.0/Tobi-Dynavox-Spellchecker-source.zip)

## TD Snap setup for eye gaze

**Daily use:** compose in TD Snap, dwell on **Spell Check** on the AAC keyboard page, then choose **Use correction** or **Keep original** in the large-button review. **Use correction** updates the same Message Window and returns to AAC. Speak when you're ready using TD Snap. There is no manual copy/paste, keyboard shortcut, or pointer countdown in this workflow. If an optional model fails, choose **Use Windows spelling** to review a Windows spelling result for the same draft.

**One-time setup** (a caregiver or setup partner can do this):

1. Save the EXE permanently in a folder on the Windows device. Launch it normally and leave **Windows Spell Checking** selected, or choose a different engine. Your explicit selection is remembered. Test any optional engine with an unsent sample and an internet connection to cache its runtime and model.
2. In TD Snap, open the keyboard page, enter a plain-text sample draft and make the Message Window visible. In Spellchecker choose **Link AAC field**, dismiss the instructions, and place the pointer on the TD Snap Message Window until setup captures it. Wait for **AAC field linked**. This checks for a uniquely identified writable Windows accessibility field.
3. In TD Snap Edit mode, add a regular button named **Spell Check** to the keyboard or message page. Set its action to **Launch Application**, choose the EXE, and in **Advanced → Command Line Arguments** enter `--check`. Put it somewhere that is easy to dwell on. The setup utility can be closed after linking.
4. Leave Edit mode. Try an unsent draft such as “I wnat a glass of water” using **Spell Check**, then **Use correction**. Check that the message changes in TD Snap and that you can still select TD Snap's normal Speak button by gaze.

The review uses regular Windows buttons. The device needs a gaze mode that can click Windows apps outside TD Snap, not only TD Snap's built-in page buttons. Tobii Dynavox documents TD Control and launch actions in the [TD Snap manual](https://download.mytobiidynavox.com/Snap/documents/TD_Snap_UsersManual_en-US.pdf). Windows desktop access and launching an EXE may be unavailable in kiosk or restricted configurations.

**Compatibility:** the eye-gaze route requires that TD Snap leaves its window active when starting the EXE and exposes a visibly available, uniquely identifiable, writable plain-text Message Window. If linking fails or the app blocks write-back, the tool reports that limitation. It will not try to simulate clicks or require you to paste as part of the gaze flow. Relink after an AAC update if the target identity changes. Don't use this plain-text route for drafts containing symbols or formatting that must be preserved.

### Other ways to use the tool

The regular EXE still has the optional **Capture field**, **Ctrl+Alt+F8**, and clipboard controls for people who want a general-purpose desktop spellchecker. **Undo AAC fix** is available in the regular window for direct replacements made from that window. These are separate from the gaze workflow.

The spellchecker never invokes Speak or sends a Speak shortcut. If AAC is configured to speak inserted text, the AAC app's own settings still apply.

## Requirements and test status

- Windows 10/11 x64, with the English (US) spelling dictionary installed. If the spelling service reports that the dictionary is missing, add English (United States) in Windows Language settings.
- The Windows engine checks spelling only. Some optional text-generation models may also change punctuation or wording. The app requires explicit review and never applies a model result automatically.
- Direct write-back, the TD Snap launch action and selection by eye gaze have not been verified on a physical Tobii Dynavox device. Test with a short unsent draft first, such as “I wnat a glass of water.”
- The executable is unsigned. Windows may display its usual warning for a new unsigned app. It does not need administrator privileges.

## Build from source

To build locally with Go 1.25.5 on 64-bit Windows:

```powershell
go test ./...
go build -buildvcs=false -trimpath -ldflags "-s -w -H windowsgui" -o Tobi-Dynavox-Spellchecker.exe .
```

Portable unit tests cover the engine catalog, model-preview validation, UTF-16 replacement offsets, stale-draft protection, read-back verification and undo. The Windows CI smoke test provisions the runtime and checks one optional model before and after blocking downloads. Windows-only tests also exercise the system dictionary and read, replace and re-read a linked field in a separate Win32 test app. Live device testing is still needed for the TD Snap direct route.

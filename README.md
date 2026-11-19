# Tobi Dynavox Spellchecker

A portable Windows 10/11 x64 spelling review tool for people composing their own AAC messages. It defaults to the installed Windows English (US) dictionary, which needs no account, model download, or internet connection. Six optional models run on your PC after a one-time download. It does not record drafts or send them anywhere. This is an independent tool, not a Tobii Dynavox product.

## Spelling engines

Choose an engine in the app, then click **Check spelling** or capture a field as usual. Windows Spell Checking is selected whenever the app starts. The first use of an optional engine automatically sets up private CPU inference libraries and downloads its weights. No separate Python installation or administrator rights are needed. Downloads are cached under `%LOCALAPPDATA%\AAC Spellchecker`. After setup, that engine works offline. Drafts are never sent to the download providers.

| Engine | Suggested PC RAM | Notes |
| --- | --- | --- |
| Windows Spell Checking (default) | 8 GB or 16 GB | Uses the installed English (US) Windows dictionary. No download. |
| [NeuSpell BERT](https://huggingface.co/murali1996/bert-base-cased-spell-correction) | 8 GB or 16 GB | Uses sentence context for word-level corrections. About 744 MB of model files. |
| [NeuSpell SC-LSTM](https://github.com/neuspell/neuspell) | 8 GB or 16 GB | Uses letters and context to suggest word-level corrections. About 452 MB. |
| [BART English Spelling Correction](https://huggingface.co/oliverguhr/spelling-correction-english-base) | 8 GB or 16 GB | Experimental text-generation model. Can change punctuation or wording. About 561 MB. |
| [T5-large-spell](https://huggingface.co/ai-forever/T5-large-spell) | 16 GB recommended | Generates a corrected version of the input. About 3 GB. |
| [SAGE mT5 Large](https://huggingface.co/ai-forever/sage-mt5-large) | 16 GB recommended | Large multilingual model with English support. About 4.9 GB. |
| [Spell Correct Transformer](https://huggingface.co/torinriley/spell-correct-transformer) | 8 GB or 16 GB | Experimental, one word at a time, no context. About 4 MB. |

These are RAM recommendations, not minimums or model download sizes. Allow additional disk space for the shared inference libraries. The large models may respond slowly on CPU. Review all model changes before applying them, especially with names, numbers or shorthand. Switch back to Windows Spell Checking if a download fails or you want to check a draft without waiting.

## Download

- [Download the portable Windows EXE](https://raw.githubusercontent.com/tbenitz/Tobi-Dynavox-Spellchecker/main/docs/downloads/Tobi-Dynavox-Spellchecker.exe)
- [Download the source ZIP](https://raw.githubusercontent.com/tbenitz/Tobi-Dynavox-Spellchecker/main/docs/downloads/Tobi-Dynavox-Spellchecker-source.zip)

## Check and replace a draft

1. Run **Tobi-Dynavox-Spellchecker.exe**. Leave it open while using your AAC app.
2. Focus the speech field and press **Ctrl+Alt+F8**. Or choose **Capture field**, then move the pointer to the speech field within five seconds. The spellchecker minimizes while waiting for the pointer.
3. Read the **Review and edit the correction** box and edit it as needed. Suggestions can get names and specialized words wrong.
4. Choose **Apply to AAC**. It attempts to replace the text in the same field it read. If the draft changed since capture, the tool will not overwrite it. **Undo AAC fix** restores the previous text if the field still contains the correction.

This tool never invokes a Speak button or sends a Speak shortcut. If your AAC app is set to speak as text is inserted, it may announce edits according to its own settings. Direct editing requires the speech field to expose a writable Windows UI Automation value. It treats the field as plain text. Use the AAC app's own editing tools for drafts containing symbols or formatting that you need to preserve.

## If TD Snap does not let another app edit the Speech field

1. In TD Snap, use the Message Bar **Share → Copy** or the **Copy Message Window Text** action.
2. In Spellchecker, choose **Paste draft**, then **Check spelling**. Review the preview and choose **Copy correction**.
3. In TD Snap, confirm the original draft has not changed. Select all text in the Message Window using a button set to **Select Text → Select all**, and use **Paste Clipboard into Message Window**. Some page sets already include a Message Window Editing page with these actions. Verify the resulting text before speaking it. If the page set inserts rather than replaces the selection, use the Message Window editing controls or undo.

The TD Snap manual describes these actions. [TD Snap Message Bar and Button Actions](https://download.mytobiidynavox.com/Snap/documents/TD_Snap_UsersManual_v1-17_en-US_12005874_WEB.pdf)

## Requirements and test status

- Windows 10/11 x64, with the English (US) spelling dictionary installed. If the spelling service reports that the dictionary is missing, add English (United States) in Windows Language settings.
- This tool corrects spelling. It does not rewrite sentence meaning, translate, or apply grammar rules.
- Direct write-back has not been verified on a physical Tobii Dynavox device. Test with a short unsent draft first, such as “I wnat a glass of water.”
- The executable is unsigned. Windows may display its usual warning for a new unsigned app. It does not need administrator privileges.

## Build from source

To build locally with Go 1.25.5 on 64-bit Windows:

```powershell
go test ./...
go build -buildvcs=false -trimpath -ldflags "-s -w -H windowsgui" -o Tobi-Dynavox-Spellchecker.exe .
```

Portable unit tests cover the engine catalog, model-preview validation, UTF-16 replacement offsets, stale-draft protection, read-back verification and undo. The Windows CI smoke test provisions the runtime and checks one optional model before and after blocking downloads. Windows-only tests exercise the system dictionary and initialize the real UI Automation service. Live device testing is still needed for the TD Snap direct route.

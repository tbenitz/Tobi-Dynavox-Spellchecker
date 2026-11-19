# First Windows build

Build a portable, offline spelling review tool for AAC drafts. Use the Windows
English dictionary, not an AI rewrite. Preview is editable and no action invokes
Speak, Enter, or any simulated keystroke. Do not run a background text monitor.

Direct route: get a user-selected editable field via Windows UI Automation.
Reject own controls, password fields and unsupported controls. Bind operations
to the captured control, compare the current value with the reviewed snapshot,
and verify any write by reading back. Offer a guarded undo.

Fallback: manual TD Snap clipboard actions. Require the person to select and
review their draft before replacing it in TD Snap. This is also the intended
route for installations that do not expose a writable accessibility control.

Implementation order: test the formatting and write guards, wire the Windows
spell checker and access bridge, then add the review interface and packaging.

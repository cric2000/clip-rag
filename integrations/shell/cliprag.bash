# Automatic clipboard-history suggestions for Bash, using bundled ble.sh.
[[ $- == *i* && -z ${_CLIPRAG_BASH_LOADED-} && ${BLE_VERSION-} ]] || return 0
ble-import lib/core-complete || return 1
_CLIPRAG_BASH_LOADED=1
_CLIPRAG_BASH_CANDIDATE=
_CLIPRAG_BASH_AUTO_SOURCES=("${_ble_complete_auto_source[@]}")
_CLIPRAG_BASH_AUTO_COMPLETE=$bleopt_complete_auto_complete
_CLIPRAG_BASH_AUTO_DELAY=$bleopt_complete_auto_delay
_CLIPRAG_BASH_ESCAPE=$bleopt_decode_isolated_esc
_CLIPRAG_BASH_AUTO_MENU=$bleopt_complete_auto_menu
declare -gA _CLIPRAG_BASH_BLE_BINDINGS=() _CLIPRAG_BASH_BLE_WRITTEN=()
ble/color/face2g auto_complete
_CLIPRAG_BASH_FACE=$ret
_cliprag_ble_bind() {
    local map=$1 key=$2 widget=$3 ret entry id
    ble/decode/keymap#load "$map" || return 1
    ble-decode-kbd "$key"
    [[ $ret =~ ^[0-9]+$ ]] || return 1
    id=$map:$ret
    builtin eval 'entry=${_ble_decode_'"$map"'_kmap_['"$ret"']-}'
    [[ ${_CLIPRAG_BASH_BLE_BINDINGS[$id]+saved} ]] || _CLIPRAG_BASH_BLE_BINDINGS[$id]=$entry
    _CLIPRAG_BASH_BLE_WRITTEN[$id]="ble/widget/$widget"
    ble-bind -m "$map" -f "$key" "$widget"
}

function ble/complete/auto-complete/source:cliprag {
    [[ ${CLIPRAG_COMPLETION_ENABLED:-1} == 1 && $_ble_decode_keymap != vi_nmap ]] || return 1
    ((_ble_edit_ind == ${#_ble_edit_str} && ${#_ble_edit_str} >= 3)) || return 1
    local candidate
    candidate=$(printf '%s' "$_ble_edit_str" | "$_CLIPRAG_COMPLETION_BIN" suggest --limit 1 2>/dev/null) || return 1
    [[ $candidate && $candidate != "$_ble_edit_str" && $candidate == "$_ble_edit_str"* ]] || return 1
    _CLIPRAG_BASH_CANDIDATE=$candidate
    ble/complete/auto-complete/enter h 0 "${candidate:${#_ble_edit_str}}" '' "$candidate"
}

_cliprag_record_use() {
    printf '%s' "$1" | "$_CLIPRAG_COMPLETION_BIN" record-use >/dev/null 2>&1 || :
}
_cliprag_bash_record_usage() {
    [[ $_ble_edit_mark_active == auto_complete && $_CLIPRAG_BASH_CANDIDATE && $_ble_edit_str == "$_CLIPRAG_BASH_CANDIDATE" ]] || return 0
    ((_ble_edit_ind == ${#_ble_edit_str})) || return 0
    _cliprag_record_use "$_CLIPRAG_BASH_CANDIDATE"
    _CLIPRAG_BASH_CANDIDATE=
}
blehook complete_insert+=_cliprag_bash_record_usage

function ble/widget/cliprag-picker {
    [[ ${CLIPRAG_COMPLETION_ENABLED:-1} == 1 ]] || return 0
    ((_ble_edit_ind == ${#_ble_edit_str})) || return 0
    local selected
    selected=$(printf '%s' "$_ble_edit_str" | "$_CLIPRAG_COMPLETION_BIN" pick 2>/dev/null) || return 0
    [[ $selected && $selected == "$_ble_edit_str"* ]] || return 0
    ble-edit/content/reset "$selected"
    _ble_edit_ind=${#_ble_edit_str}
    ble/textarea#invalidate
    _cliprag_record_use "$selected"
}

_ble_complete_auto_source=(cliprag)
bleopt complete_auto_complete=1 complete_auto_delay=120 complete_auto_menu= decode_isolated_esc=esc
# Keep ordinary command-line colors; only the unaccepted suffix is faded.
if [[ ${_CLIPRAG_BASH_OWNS_BLE-} ]]; then
    bleopt highlight_syntax= highlight_filename= highlight_variable=
fi
ble-face auto_complete=fg=242
ble/decode/keymap#load auto_complete
# Enter and Ctrl+Enter must never accept an unseen suffix and execute it.
_cliprag_ble_bind auto_complete RET auto_complete/cancel-default
_cliprag_ble_bind auto_complete C-m auto_complete/cancel-default
_cliprag_ble_bind auto_complete C-j auto_complete/cancel-default
_cliprag_ble_bind auto_complete C-RET auto_complete/insert
_cliprag_ble_bind auto_complete ESC auto_complete/cancel
_cliprag_ble_bind auto_complete C-e auto_complete/cancel-default
_cliprag_ble_bind auto_complete end auto_complete/cancel-default
_cliprag_ble_bind emacs M-/ cliprag-picker
_cliprag_ble_bind vi_imap M-/ cliprag-picker

# Recompute after deletions as well as the editor's normal self-insert idle hook.
_cliprag_bash_delete_idle() {
    ble/util/idle.wait-user-input
    [[ ${CLIPRAG_COMPLETION_ENABLED:-1} == 1 && $_ble_edit_mark_active != auto_complete ]] || return 0
    case $_ble_decode_widget_last in
        ble/widget/delete-backward-char|ble/widget/delete-forward-char|ble/widget/delete-region-or-backward-char) ;;
        *) return 0 ;;
    esac
    [[ $_ble_decode_keymap == emacs || $_ble_decode_keymap == vi_imap ]] || return 0
    ble/util/idle.sleep-until "$((_ble_idle_clock_start+bleopt_complete_auto_delay))" checked && return 0
    ble/complete/auto-complete.impl
}
ble/util/idle.push-background _cliprag_bash_delete_idle

cliprag_completion_disable() {
    if [[ $_ble_edit_mark_active == auto_complete ]]; then
        ble/widget/auto_complete/cancel
    fi
    local id map code entry saved
    for id in "${!_CLIPRAG_BASH_BLE_BINDINGS[@]}"; do
        map=${id%%:*} code=${id#*:}
        builtin eval 'entry=${_ble_decode_'"$map"'_kmap_['"$code"']-}'
        [[ ${entry#*:} == "${_CLIPRAG_BASH_BLE_WRITTEN[$id]}" ]] || continue
        saved=${_CLIPRAG_BASH_BLE_BINDINGS[$id]}
        if [[ $saved == *:* && ${saved#*:} ]]; then
            ble-decode-key/bind "$map" "$code" "${saved#*:}"
        else
            ble-decode-key/unbind "$map" "$code"
        fi
    done
    ble/util/idle.cancel _cliprag_bash_delete_idle
    blehook complete_insert-=_cliprag_bash_record_usage
    _ble_faces[_ble_faces__auto_complete]=$_CLIPRAG_BASH_FACE
    _ble_complete_auto_source=("${_CLIPRAG_BASH_AUTO_SOURCES[@]}")
    bleopt complete_auto_complete="$_CLIPRAG_BASH_AUTO_COMPLETE" complete_auto_delay="$_CLIPRAG_BASH_AUTO_DELAY" decode_isolated_esc="$_CLIPRAG_BASH_ESCAPE" complete_auto_menu="$_CLIPRAG_BASH_AUTO_MENU"
    unset _CLIPRAG_BASH_LOADED
    if [[ ${_CLIPRAG_BASH_OWNS_BLE-} ]]; then
        ble-detach
    fi
}

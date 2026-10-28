# Automatic inline clipboard-history suggestions for interactive Zsh.
[[ -o interactive && -z ${_CLIPRAG_ZSH_LOADED-} ]] || return 0
zmodload zsh/zle 2>/dev/null || return 1
zmodload zsh/zleparameter 2>/dev/null || return 1
autoload -Uz add-zle-hook-widget
(( $+functions[add-zle-hook-widget] )) || return 1
typeset -g _CLIPRAG_ZSH_LOADED=1
typeset -gA _CLIPRAG_ZSH_BINDING _CLIPRAG_ZSH_ESCAPE_BINDING
typeset -g _CLIPRAG_ZSH_PREFIX='' _CLIPRAG_ZSH_CANDIDATE='' _CLIPRAG_ZSH_SUFFIX='' _CLIPRAG_ZSH_MARK='' _CLIPRAG_ZSH_DISMISSED=''

_cliprag_record_use() {
    printf '%s' "$1" | "$_CLIPRAG_COMPLETION_BIN" record-use >/dev/null 2>&1 || :
}
_cliprag_auto_clear() {
    emulate -L zsh
    [[ -z $_CLIPRAG_ZSH_SUFFIX || $POSTDISPLAY != $_CLIPRAG_ZSH_SUFFIX ]] || POSTDISPLAY=''
    if [[ -n $_CLIPRAG_ZSH_MARK ]]; then
        local entry
        local -a kept=()
        for entry in "${region_highlight[@]}"; do
            [[ $entry == "$_CLIPRAG_ZSH_MARK" ]] || kept+=("$entry")
        done
        region_highlight=("${kept[@]}")
    fi
    _CLIPRAG_ZSH_SUFFIX=''
    _CLIPRAG_ZSH_MARK=''
}
_cliprag_auto_reset() {
    _cliprag_auto_clear
    _CLIPRAG_ZSH_PREFIX=''
    _CLIPRAG_ZSH_CANDIDATE=''
    _CLIPRAG_ZSH_DISMISSED=''
}
_cliprag_auto_render() {
    emulate -L zsh
    _cliprag_auto_clear
    [[ $BUFFER == "$_CLIPRAG_ZSH_DISMISSED" ]] || _CLIPRAG_ZSH_DISMISSED=''
    [[ ${CLIPRAG_COMPLETION_ENABLED:-1} == 1 && -z $RBUFFER && ${#BUFFER} -ge 3 && $KEYMAP != vicmd && $BUFFER != "$_CLIPRAG_ZSH_DISMISSED" && -z $POSTDISPLAY ]] || return 0
    if [[ $_CLIPRAG_ZSH_PREFIX != $BUFFER ]]; then
        _CLIPRAG_ZSH_CANDIDATE=$(print -rn -- "$BUFFER" | "$_CLIPRAG_COMPLETION_BIN" suggest --limit 1 2>/dev/null) || _CLIPRAG_ZSH_CANDIDATE=''
        _CLIPRAG_ZSH_PREFIX=$BUFFER
    fi
    [[ -n $_CLIPRAG_ZSH_CANDIDATE && $_CLIPRAG_ZSH_CANDIDATE != "$BUFFER" && $_CLIPRAG_ZSH_CANDIDATE == "$BUFFER"* ]] || return 0
    _CLIPRAG_ZSH_SUFFIX=${_CLIPRAG_ZSH_CANDIDATE:${#BUFFER}}
    POSTDISPLAY=$_CLIPRAG_ZSH_SUFFIX
    _CLIPRAG_ZSH_MARK="${#BUFFER} $((${#BUFFER} + ${#POSTDISPLAY})) fg=242 memo=cliprag"
    region_highlight+=("$_CLIPRAG_ZSH_MARK")
}
_cliprag_auto_forward() {
    emulate -L zsh
    if [[ ${CLIPRAG_COMPLETION_ENABLED:-1} == 1 && -z $RBUFFER && -n $_CLIPRAG_ZSH_SUFFIX && $_CLIPRAG_ZSH_PREFIX == "$BUFFER" && $POSTDISPLAY == "$_CLIPRAG_ZSH_SUFFIX" ]]; then
        BUFFER=$_CLIPRAG_ZSH_CANDIDATE
        CURSOR=${#BUFFER}
        _cliprag_auto_clear
        _cliprag_record_use "$BUFFER"
    else
        _cliprag_auto_clear
        zle "cliprag-original-$WIDGET" -- "$@"
    fi
}
_cliprag_auto_dismiss() {
    emulate -L zsh
    _CLIPRAG_ZSH_DISMISSED=$BUFFER
    _cliprag_auto_clear
    if [[ $KEYMAP == viins ]]; then zle vi-cmd-mode; fi
}
_cliprag_complete() {
    emulate -L zsh
    local selected
    _cliprag_auto_clear
    [[ ${CLIPRAG_COMPLETION_ENABLED:-1} == 1 && -z $RBUFFER ]] || return 0
    selected=$(print -rn -- "$BUFFER" | "$_CLIPRAG_COMPLETION_BIN" pick 2>/dev/null) || { zle -R; return 0; }
    [[ -n $selected && $selected == "$BUFFER"* ]] || return 0
    BUFFER=$selected
    CURSOR=${#BUFFER}
    zle -R
    _cliprag_record_use "$BUFFER"
}
zle -N cliprag-complete _cliprag_complete
zle -N cliprag-dismiss _cliprag_auto_dismiss
for _cliprag_widget in forward-char vi-forward-char; do
    zle -A "$_cliprag_widget" "cliprag-original-$_cliprag_widget"
    zle -N "$_cliprag_widget" _cliprag_auto_forward
done
add-zle-hook-widget line-pre-redraw _cliprag_auto_render
add-zle-hook-widget line-init _cliprag_auto_reset
add-zle-hook-widget line-finish _cliprag_auto_reset

cliprag_completion_disable() {
    emulate -L zsh
    local map binding widget
    _cliprag_auto_clear
    add-zle-hook-widget -d line-pre-redraw _cliprag_auto_render
    add-zle-hook-widget -d line-init _cliprag_auto_reset
    add-zle-hook-widget -d line-finish _cliprag_auto_reset
    for widget in forward-char vi-forward-char; do
        if [[ ${widgets[$widget]} == user:_cliprag_auto_forward ]]; then
            zle -A "cliprag-original-$widget" "$widget"
        fi
        zle -D "cliprag-original-$widget"
    done
    for map in emacs viins; do
        binding=$(bindkey -M "$map" '^[/' 2>/dev/null)
        [[ $binding != *' cliprag-complete' ]] || bindkey -M "$map" '^[/' "${_CLIPRAG_ZSH_BINDING[$map]:-undefined-key}"
        binding=$(bindkey -M "$map" '^[' 2>/dev/null)
        [[ $binding != *' cliprag-dismiss' ]] || bindkey -M "$map" '^[' "${_CLIPRAG_ZSH_ESCAPE_BINDING[$map]:-undefined-key}"
    done
    zle -D cliprag-complete cliprag-dismiss
    unset _CLIPRAG_ZSH_LOADED
}
for _cliprag_map in emacs viins; do
    _cliprag_binding=$(bindkey -M "$_cliprag_map" '^[/' 2>/dev/null)
    _CLIPRAG_ZSH_BINDING[$_cliprag_map]=${_cliprag_binding##* }
    _cliprag_binding=$(bindkey -M "$_cliprag_map" '^[' 2>/dev/null)
    _CLIPRAG_ZSH_ESCAPE_BINDING[$_cliprag_map]=${_cliprag_binding##* }
    bindkey -M "$_cliprag_map" '^[/' cliprag-complete
    bindkey -M "$_cliprag_map" '^[' cliprag-dismiss
done
unset _cliprag_map _cliprag_binding _cliprag_widget

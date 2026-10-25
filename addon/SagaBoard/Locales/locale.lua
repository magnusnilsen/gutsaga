-- Resolves a catalog key. Missing keys fall back to English.
-- SagaBoardLanguage is "en", "nb", or nil (then GetLocale()).
local function catalog(lang)
    local all = SagaBoardLocale or {}
    if lang == "nb" and type(all.nb) == "table" then
        return all.nb
    end
    return all.en or {}
end

local function clientLang()
    local set = SagaBoardLanguage
    if set == "en" or set == "nb" then
        return set
    end
    local loc = "enUS"
    if type(GetLocale) == "function" then
        loc = GetLocale() or loc
    end
    if loc == "nbNO" or loc == "noNO" then
        return "nb"
    end
    return "en"
end

function SagaBoard_L(key)
    if type(key) ~= "string" or key == "" then
        return ""
    end
    local lang = clientLang()
    local value = catalog(lang)[key]
    if value == nil and lang ~= "en" then
        local en = catalog("en")
        value = en[key]
    end
    if value == nil then
        return key
    end
    return value
end

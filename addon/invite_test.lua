-- Invite dialog outcomes, snooze across a simulated reload, and English fallback.
local dir = arg[0]:match("^(.*)/") or "."

dofile(dir .. "/SagaBoard/Locales/enUS.lua")
dofile(dir .. "/SagaBoard/Locales/nbNO.lua")
dofile(dir .. "/SagaBoard/Locales/locale.lua")
local Invite = dofile(dir .. "/SagaBoard/SagaBoardInvite.lua")

local function assertTrue(cond, msg)
    if not cond then
        error(msg or "assertion failed", 2)
    end
end

local id = "11111111-1111-1111-1111-111111111111"
assertTrue(Invite.line("accept", id) == "[SagaBoard] INVITE accept|" .. id, "accept line")
assertTrue(Invite.line("decline", id) == "[SagaBoard] INVITE decline|" .. id, "decline line")
assertTrue(Invite.line("later", id) == nil, "later is client-only")

local db = {}
local now = 1700000000
Invite.snooze(db, id, now)
assertTrue(Invite.snoozed(db, id, now + 10), "snoozed immediately")
-- /reload keeps SavedVariables; the same table is what the addon reads next.
local reloaded = db
assertTrue(Invite.snoozed(reloaded, id, now + 12 * 3600 - 1), "snooze survives reload")
assertTrue(not Invite.snoozed(reloaded, id, now + 12 * 3600 + 1), "snooze expires")

local data = {
    invites = {
        { id = id, name = "Magni", realm = "Everlook", group_name = "Tutorial", kind = "join" },
    },
}
local character = { name = "Magni", realm = "Everlook" }
local prompt = Invite.prompt({}, character, data, now)
assertTrue(prompt and prompt.key == "invite.join" and prompt.group == "Tutorial", "join prompt")
Invite.snooze(db, id, now)
assertTrue(Invite.prompt(db, character, data, now + 5) == nil, "snooze hides the dialog")

SagaBoardLanguage = "nb"
assertTrue(SagaBoard_L("invite.join") == "Bli med i %s?", "nb join")
assertTrue(SagaBoard_L("invite.accept") == "Ja", "nb accept")
assertTrue(SagaBoard_L("invite.decline") == "Nei", "nb decline")
assertTrue(SagaBoard_L("invite.later") == "Ikke nå", "nb later")

SagaBoardLocale.en["test.only_en"] = "English fallback"
SagaBoardLanguage = "nb"
assertTrue(SagaBoard_L("test.only_en") == "English fallback", "missing nb key uses English")

SagaBoardLanguage = "en"
assertTrue(SagaBoard_L("invite.join") == "Join %s?", "en join")

for key in pairs(SagaBoardLocale.nb) do
    if SagaBoardLocale.en[key] == nil then
        error("nb key missing from en: " .. key)
    end
end

print("invite_test ok")

-- Invite dialog outcomes, snooze across a simulated reload, and English fallback.
local dir = arg[0]:match("^(.*)/") or "."

dofile(dir .. "/OurSagas/Locales/enUS.lua")
dofile(dir .. "/OurSagas/Locales/nbNO.lua")
dofile(dir .. "/OurSagas/Locales/locale.lua")
local Invite = dofile(dir .. "/OurSagas/OurSagasInvite.lua")

local function assertTrue(cond, msg)
    if not cond then
        error(msg or "assertion failed", 2)
    end
end

local id = "11111111-1111-1111-1111-111111111111"
assertTrue(Invite.line("accept", id) == "[OurSagas] INVITE accept|" .. id, "accept line")
assertTrue(Invite.line("decline", id) == "[OurSagas] INVITE decline|" .. id, "decline line")
assertTrue(Invite.line("later", id) == nil, "later is client-only")

local db = {}
local now = 1700000000
Invite.snooze(db, id, now)
assertTrue(Invite.snoozed(db, id, now + 10), "snoozed immediately")
-- /reload keeps SavedVariables; the same table is what the addon reads next.
local reloaded = db
assertTrue(Invite.snoozed(reloaded, id, now + 12 * 3600 - 1), "snooze survives reload")
assertTrue(not Invite.snoozed(reloaded, id, now + 12 * 3600 + 1), "snooze expires")

local id2 = "22222222-2222-2222-2222-222222222222"
local data = {
    invites = {
        { id = id, name = "Magni", realm = "Everlook", group_name = "Tutorial", kind = "join" },
        { id = id2, name = "Magni", realm = "Everlook", group_name = "Gutar", kind = "include" },
    },
}
local character = { name = "Magni", realm = "Everlook" }
local prompt = Invite.prompt({}, character, data, now)
assertTrue(prompt and prompt.key == "invite.join" and prompt.group == "Tutorial", "join prompt")
Invite.markAnswered(db, id)
prompt = Invite.prompt(db, character, data, now)
assertTrue(prompt and prompt.key == "invite.include" and prompt.group == "Gutar", "next invite after answer")
Invite.snooze(db, id2, now)
assertTrue(Invite.prompt(db, character, data, now + 5) == nil, "snooze hides the dialog")

OurSagasLanguage = "nb"
assertTrue(OurSagas_L("invite.join") == "Bli med i %s?", "nb join")
assertTrue(OurSagas_L("invite.accept") == "Ja", "nb accept")
assertTrue(OurSagas_L("invite.decline") == "Nei", "nb decline")
assertTrue(OurSagas_L("invite.later") == "Ikke nå", "nb later")

OurSagasLocale.en["test.only_en"] = "English fallback"
OurSagasLanguage = "nb"
assertTrue(OurSagas_L("test.only_en") == "English fallback", "missing nb key uses English")

OurSagasLanguage = "en"
assertTrue(OurSagas_L("invite.join") == "Join %s?", "en join")

for key in pairs(OurSagasLocale.nb) do
    if OurSagasLocale.en[key] == nil then
        error("nb key missing from en: " .. key)
    end
end

print("invite_test ok")

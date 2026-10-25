-- Invite prompts and the chat lines the companion posts.
-- The addon has no HTTP. SavedVariables hold the 12-hour snooze across /reload.
local M = {}

local SNOOZE = 12 * 60 * 60

local function same(a, b)
    if type(a) ~= "string" or type(b) ~= "string" then
        return false
    end
    return string.lower(a) == string.lower(b)
end

function M.line(action, id)
    if type(id) ~= "string" or id == "" then
        return nil
    end
    if action ~= "accept" and action ~= "decline" then
        return nil
    end
    return "[SagaBoard] INVITE " .. action .. "|" .. id
end

function M.snooze(db, id, now)
    if type(db) ~= "table" or type(id) ~= "string" or id == "" then
        return
    end
    if type(db.invite_snooze) ~= "table" then
        db.invite_snooze = {}
    end
    local at = tonumber(now) or 0
    db.invite_snooze[id] = at + SNOOZE
end

function M.snoozed(db, id, now)
    if type(db) ~= "table" or type(db.invite_snooze) ~= "table" then
        return false
    end
    local untilAt = tonumber(db.invite_snooze[id])
    local at = tonumber(now) or 0
    return untilAt ~= nil and untilAt > at
end

-- prompt returns nil or { id, key, group, character }.
-- key is invite.join or invite.include.
function M.prompt(db, character, data, now)
    if type(data) ~= "table" or type(data.invites) ~= "table" then
        return nil
    end
    local name = character and character.name or ""
    local realm = character and character.realm or ""
    if name == "" then
        return nil
    end
    for i = 1, #data.invites do
        local inv = data.invites[i]
        if type(inv) == "table" and type(inv.id) == "string" and inv.id ~= "" then
            if not M.snoozed(db, inv.id, now) and same(inv.name, name) and same(inv.realm or "", realm) then
                local key = "invite.join"
                if inv.kind == "include" then
                    key = "invite.include"
                end
                return {
                    id = inv.id,
                    key = key,
                    group = inv.group_name or "",
                    character = name,
                }
            end
        end
    end
    return nil
end

SagaBoardInvite = M
return M

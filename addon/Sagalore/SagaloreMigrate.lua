-- One-time copy of SavedVariables from the OurSagas addon name.
-- WoW only loads WTF SavedVariables for the current addon folder. The
-- companion copies OurSagas.lua to Sagalore.lua before this runs. If both
-- addons were loaded in one session, OurSagasDB is still a global and this
-- fills SagaloreDB when the new table has no progress yet.

local M = {}

function M.hasProgress(db)
    if type(db) ~= "table" then
        return false
    end
    if type(db.completions) == "table" and next(db.completions) ~= nil then
        return true
    end
    if type(db.objectives) == "table" and next(db.objectives) ~= nil then
        return true
    end
    if type(db.pending_resets) == "table" and next(db.pending_resets) ~= nil then
        return true
    end
    if type(db.pending_invites) == "table" and next(db.pending_invites) ~= nil then
        return true
    end
    if type(db.seen_notice) == "string" and db.seen_notice ~= "" then
        return true
    end
    return false
end

function M.apply(current, legacy)
    if type(legacy) ~= "table" then
        return current
    end
    if type(current) ~= "table" or (not M.hasProgress(current) and M.hasProgress(legacy)) then
        return legacy
    end
    return current
end

SagaloreMigrate = M
return M

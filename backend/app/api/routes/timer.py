from typing import Optional
import uuid
from fastapi import APIRouter, Depends, status
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, delete

from app.api.deps import get_current_user, get_db, owned_skill
from app.core.exceptions import AppError
from app.models import User, SkillTimer
from app.schemas import SkillTimerIn, SkillTimerOut

router = APIRouter()


@router.get("/{skill_id}/timer", response_model=Optional[SkillTimerOut])
async def get_timer(
    skill_id: uuid.UUID,
    current_user: User = Depends(get_current_user),
    db: AsyncSession = Depends(get_db),
):
    await owned_skill(skill_id, current_user, db)

    timer_stmt = select(SkillTimer).where(
        SkillTimer.skill_id == skill_id, SkillTimer.user_id == current_user.id
    )
    timer_result = await db.execute(timer_stmt)
    timer = timer_result.scalars().first()

    return timer


@router.post("/{skill_id}/timer", response_model=SkillTimerOut)
async def set_timer(
    skill_id: uuid.UUID,
    data: SkillTimerIn,
    current_user: User = Depends(get_current_user),
    db: AsyncSession = Depends(get_db),
):
    if data.from_date >= data.to_date:
        raise AppError(400, "invalid_dates", "to_date must be strictly after from_date")

    await owned_skill(skill_id, current_user, db)

    # Delete existing timer if any
    del_stmt = delete(SkillTimer).where(
        SkillTimer.skill_id == skill_id, SkillTimer.user_id == current_user.id
    )
    await db.execute(del_stmt)

    timer = SkillTimer(
        user_id=current_user.id,
        skill_id=skill_id,
        from_date=data.from_date,
        to_date=data.to_date,
    )
    db.add(timer)
    await db.commit()
    await db.refresh(timer)
    return timer


@router.delete("/{skill_id}/timer", status_code=status.HTTP_204_NO_CONTENT)
async def delete_timer(
    skill_id: uuid.UUID,
    current_user: User = Depends(get_current_user),
    db: AsyncSession = Depends(get_db),
):
    await owned_skill(skill_id, current_user, db)

    del_stmt = delete(SkillTimer).where(
        SkillTimer.skill_id == skill_id, SkillTimer.user_id == current_user.id
    )
    await db.execute(del_stmt)
    await db.commit()

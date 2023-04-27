import React from "react";
import Badge from "@mui/material/Badge";
import { styled } from "@mui/system";
import ShoppingCartIcon from "@mui/icons-material/ShoppingCart";
import Popover from "@mui/material/Popover";
import Typography from "@mui/material/Typography";
import { Link } from "react-router-dom";
import {useCart} from "./useCart";

const Box = styled("div")({});
const CartIconBox = styled("button")({
  border: "none",
  background: "none",
  cursor: "pointer",
  padding: "0",
  margin: "0",
});

interface CartItem {
  id: string;
  quantity: number;
}

interface Price {
  amount: number;
  currency: string;
}

export interface CartIconProps {
  items? : CartItem[];
  itemsCount?: number;
}

export function CartIcon(props : CartIconProps) {
  const [anchorEl, setAnchorEl] =
    React.useState<HTMLButtonElement | null>(null);

  const handleClick = (event: React.MouseEvent<HTMLButtonElement>) => {
    setAnchorEl(event.currentTarget);
  };

  const handleClose = () => {
    setAnchorEl(null);
  };

  const open = Boolean(anchorEl);
  const id = open ? "simple-popover" : undefined;

  let counter = 0;
  if (props.itemsCount) {
    counter = props.itemsCount;
  }

  return (
    <Box>
      <Badge badgeContent={counter}>
        <CartIconBox onClick={handleClick}>
          <ShoppingCartIcon color="action" />
        </CartIconBox>
      </Badge>

      <Popover
        id={id}
        open={open}
        anchorEl={anchorEl}
        onClose={handleClose}
        anchorOrigin={{
          vertical: "bottom",
          horizontal: "left",
        }}
      >
        {counter > 0 ? (
          <Typography sx={{ p: 2 }}>
            <Link to="/cart">Go to cart</Link>
          </Typography>
        ) : (
          <Typography sx={{ p: 2 }}>The cart is empty</Typography>
        )}
      </Popover>
    </Box>
  );
}

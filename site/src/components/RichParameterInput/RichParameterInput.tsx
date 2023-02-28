import FormControlLabel from "@material-ui/core/FormControlLabel"
import Radio from "@material-ui/core/Radio"
import RadioGroup from "@material-ui/core/RadioGroup"
import { makeStyles } from "@material-ui/core/styles"
import TextField from "@material-ui/core/TextField"
import { Stack } from "components/Stack/Stack"
import { FC, useState } from "react"
import { TemplateVersionParameter } from "../../api/typesGenerated"
import { colors } from "theme/colors"
import { MemoizedMarkdown } from "components/Markdown/Markdown"
import Lock from "@material-ui/icons/Lock"
import { Button } from "@material-ui/core"

const isBoolean = (parameter: TemplateVersionParameter) => {
  return parameter.type === "bool"
}

export interface ParameterLabelProps {
  index: number
  parameter: TemplateVersionParameter
}

const ParameterLabel: FC<ParameterLabelProps> = ({ index, parameter }) => {
  const styles = useStyles()

  return (
    <span>
      <span className={styles.labelNameWithIcon}>
        {parameter.icon && (
          <img
            className={styles.icon}
            alt="Parameter icon"
            src={parameter.icon}
            style={{
              pointerEvents: "none",
            }}
          />
        )}
        <span className={styles.labelName}>
          <label htmlFor={`rich_parameter_values[${index}].value`}>
            {parameter.name}
          </label>
        </span>
        {!parameter.mutable && (
          <div className={styles.labelImmutable}>
            <Lock />
            This parameter cannot be changed after creation.
          </div>
        )}
      </span>
      {parameter.description && (
        <span className={styles.labelDescription}>
          <MemoizedMarkdown>{parameter.description}</MemoizedMarkdown>
        </span>
      )}
    </span>
  )
}

export interface RichParameterInputProps {
  index: number
  disabled?: boolean
  parameter: TemplateVersionParameter
  onChange: (value: string) => void
  initialValue?: string
}

export const RichParameterInput: FC<RichParameterInputProps> = ({
  index,
  disabled,
  onChange,
  parameter,
  initialValue,
  ...props
}) => {
  const styles = useStyles()

  return (
    <Stack direction="column" spacing={0.75}>
      <ParameterLabel index={index} parameter={parameter} />
      <div className={styles.input}>
        <RichParameterField
          {...props}
          index={index}
          disabled={disabled}
          onChange={onChange}
          parameter={parameter}
          initialValue={initialValue}
        />
      </div>
    </Stack>
  )
}

const RichParameterField: React.FC<RichParameterInputProps> = ({
  disabled,
  onChange,
  parameter,
  initialValue,
  ...props
}) => {
  const [parameterValue, setParameterValue] = useState(initialValue)
  const styles = useStyles()

  if (isBoolean(parameter)) {
    return (
      <RadioGroup
        defaultValue={parameterValue}
        onChange={(event) => {
          onChange(event.target.value)
        }}
      >
        <FormControlLabel
          disabled={disabled}
          value="true"
          control={<Radio color="primary" size="small" disableRipple />}
          label="True"
        />
        <FormControlLabel
          disabled={disabled}
          value="false"
          control={<Radio color="primary" size="small" disableRipple />}
          label="False"
        />
      </RadioGroup>
    )
  }

  if (parameter.options.length > 0) {
    return (
      <div className={styles.optionGrid}>
        {parameter.options.map((option) => (
          <Button
            key={option.name}
            onClick={() => {
              setParameterValue(option.value)
              onChange(option.value)
            }}
            className={`${styles.optionButton} ${
              parameterValue === option.value ? "active" : ""
            }`}
          >
            {option.icon && (
              <img
                className={styles.optionIcon}
                alt="Parameter icon"
                src={option.icon}
                style={{
                  pointerEvents: "none",
                }}
              />
            )}
            {option.name}
          </Button>
        ))}
      </div>
    )
  }

  // A text field can technically handle all cases!
  // As other cases become more prominent (like filtering for numbers),
  // we should break this out into more finely scoped input fields.
  return (
    <TextField
      {...props}
      type={parameter.type}
      size="small"
      disabled={disabled}
      placeholder={parameter.default_value}
      value={parameterValue}
      onChange={(event) => {
        setParameterValue(event.target.value)
        onChange(event.target.value)
      }}
    />
  )
}

const iconSize = 20
const optionIconSize = 24

const useStyles = makeStyles((theme) => ({
  labelName: {
    fontSize: 16,
    fontWeight: 500,
    color: theme.palette.text.primary,
    display: "block",
  },
  labelNameWithIcon: {
    display: "flex",
    alignItems: "center",
    gap: 6,
  },
  labelDescription: {
    fontSize: 14,
    color: theme.palette.text.secondary,
    display: "block",
    fontWeight: 400,
  },
  labelImmutable: {
    color: colors.gray[7],
    display: "flex",
    alignItems: "center",
    fontSize: 12,
    marginLeft: 4,

    "& svg": {
      width: 16,
      height: 16,
      marginRight: 2,
    },
  },
  input: {
    display: "flex",
    flexDirection: "column",
  },
  checkbox: {
    display: "flex",
    alignItems: "center",
    gap: theme.spacing(1),
  },
  icon: {
    maxHeight: iconSize,
    width: iconSize,
  },
  optionGrid: {
    display: "grid",
    gridTemplateColumns: "1fr 1fr 1fr",
    gap: 16,
  },
  optionIcon: {
    maxHeight: optionIconSize,
    width: optionIconSize,
    marginRight: theme.spacing(1.0),
  },
  optionButton: {
    height: "unset",
    minHeight: 52,

    "&.active": {
      outline: `2px solid ${theme.palette.primary.main}`,
    },
  },
}))
